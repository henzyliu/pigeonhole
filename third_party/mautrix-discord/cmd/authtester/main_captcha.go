package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-discord/pkg/discordauth"
)

var errCaptchaBrowserCanceled = errors.New("captcha browser flow canceled")

//go:embed main_captcha.html
var captchaPageHTML string

type captchaServer struct {
	mu      sync.Mutex
	log     zerolog.Logger
	handler http.Handler
	server  *http.Server
	ln      net.Listener
	baseURL string
	active  *activeCaptcha
}

type activeCaptcha struct {
	challenge browserCaptchaChallenge
	resultCh  chan captchaBrowserResult
}

type browserCaptchaChallenge struct {
	ID        string                     `json:"id"`
	Service   discordauth.CaptchaService `json:"service"`
	SiteKey   string                     `json:"site_key"`
	RqData    string                     `json:"rqdata,omitempty"`
	Invisible bool                       `json:"invisible"`
}

type captchaBrowserResult struct {
	token string
	err   error
}

type captchaSolveRequest struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type captchaCancelRequest struct {
	ID string `json:"id"`
}

type captchaErrorRequest struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type captchaErrorResponse struct {
	Error string `json:"error"`
}

func newCaptchaServer(log zerolog.Logger) *captchaServer {
	cs := &captchaServer{log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("/", cs.handlePage)
	mux.HandleFunc("/api/challenge", cs.handleChallenge)
	mux.HandleFunc("/api/solve", cs.handleSolve)
	mux.HandleFunc("/api/cancel", cs.handleCancel)
	mux.HandleFunc("/api/error", cs.handleError)
	cs.handler = mux
	return cs
}

func supportsBrowserCaptcha(captcha *discordauth.Captcha) bool {
	return captcha != nil &&
		captcha.Service == discordauth.CaptchaServiceHCaptcha &&
		captcha.SiteKey != nil &&
		strings.TrimSpace(*captcha.SiteKey) != ""
}

func (cs *captchaServer) startChallenge(captcha *discordauth.Captcha) (string, func(context.Context) (string, error), error) {
	if !supportsBrowserCaptcha(captcha) {
		return "", nil, fmt.Errorf("browser flow only supports hcaptcha challenges with a sitekey")
	}
	if err := cs.ensureStarted(); err != nil {
		return "", nil, err
	}

	challenge := &activeCaptcha{
		challenge: browserCaptchaChallenge{
			ID:        uuid.NewString(),
			Service:   captcha.Service,
			SiteKey:   strings.TrimSpace(*captcha.SiteKey),
			Invisible: captcha.Invisible,
		},
		resultCh: make(chan captchaBrowserResult, 1),
	}
	if captcha.RqData != nil {
		challenge.challenge.RqData = *captcha.RqData
	}

	cs.mu.Lock()
	if cs.active != nil {
		cs.log.Warn().
			Str("replaced_challenge_id", cs.active.challenge.ID).
			Msg("Replacing active CAPTCHA challenge before it was resolved")
	}
	cs.active = challenge
	pageURL := cs.baseURL
	cs.mu.Unlock()

	cs.log.Info().
		Str("challenge_id", challenge.challenge.ID).
		Str("captcha_service", string(challenge.challenge.Service)).
		Bool("captcha_invisible", challenge.challenge.Invisible).
		Bool("captcha_has_rqdata", challenge.challenge.RqData != "").
		Str("page_url", pageURL).
		Msg("Started local CAPTCHA challenge")

	wait := func(ctx context.Context) (string, error) {
		defer cs.clearActiveChallenge(challenge.challenge.ID)

		select {
		case result := <-challenge.resultCh:
			if result.err != nil {
				cs.log.Warn().
					Str("challenge_id", challenge.challenge.ID).
					Err(result.err).
					Msg("Local CAPTCHA challenge completed with error")
				return "", result.err
			}
			if result.token == "" {
				return "", fmt.Errorf("browser page returned an empty CAPTCHA token")
			}
			cs.log.Info().
				Str("challenge_id", challenge.challenge.ID).
				Int("token_length", len(result.token)).
				Msg("Local CAPTCHA challenge returned a token")
			return result.token, nil
		case <-ctx.Done():
			cs.log.Warn().
				Str("challenge_id", challenge.challenge.ID).
				Err(ctx.Err()).
				Msg("Stopped waiting for local CAPTCHA challenge")
			return "", ctx.Err()
		}
	}

	return pageURL, wait, nil
}

func (cs *captchaServer) ensureStarted() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.server != nil {
		return nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to listen on 127.0.0.1: %w", err)
	}

	addr := ln.Addr().(*net.TCPAddr)
	server := &http.Server{
		Handler:           cs.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	cs.ln = ln
	cs.server = server
	cs.baseURL = fmt.Sprintf("http://localhost:%d/", addr.Port)

	cs.log.Info().
		Str("listen_addr", ln.Addr().String()).
		Str("page_url", cs.baseURL).
		Msg("Started local CAPTCHA server")

	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cs.log.Error().Err(err).Msg("Local CAPTCHA server stopped unexpectedly")
			cs.failActiveChallenge(fmt.Errorf("captcha server stopped unexpectedly: %w", err))
		}
	}()

	return nil
}

func (cs *captchaServer) Close(ctx context.Context) error {
	cs.mu.Lock()
	server := cs.server
	cs.server = nil
	cs.ln = nil
	cs.baseURL = ""
	cs.active = nil
	cs.mu.Unlock()

	if server == nil {
		return nil
	}

	cs.log.Info().Msg("Shutting down local CAPTCHA server")
	return server.Shutdown(ctx)
}

func (cs *captchaServer) clearActiveChallenge(id string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.active != nil && cs.active.challenge.ID == id {
		cs.active = nil
	}
}

func (cs *captchaServer) failActiveChallenge(err error) {
	cs.log.Error().Err(err).Msg("Failing active CAPTCHA challenge")
	cs.mu.Lock()
	active := cs.active
	cs.active = nil
	cs.mu.Unlock()

	if active == nil {
		return
	}

	select {
	case active.resultCh <- captchaBrowserResult{err: err}:
	default:
	}
}

func (cs *captchaServer) resolveActiveChallenge(id string, result captchaBrowserResult) error {
	cs.mu.Lock()
	active := cs.active
	if active == nil {
		cs.mu.Unlock()
		cs.log.Warn().
			Str("challenge_id", id).
			Msg("Attempted to resolve CAPTCHA challenge, but none is active")
		return fmt.Errorf("no active captcha challenge")
	}
	if active.challenge.ID != id {
		cs.mu.Unlock()
		cs.log.Warn().
			Str("challenge_id", id).
			Str("active_challenge_id", active.challenge.ID).
			Msg("Attempted to resolve a stale CAPTCHA challenge")
		return fmt.Errorf("captcha challenge is no longer current")
	}
	cs.active = nil
	cs.mu.Unlock()

	select {
	case active.resultCh <- result:
		return nil
	default:
		cs.log.Warn().
			Str("challenge_id", id).
			Msg("CAPTCHA challenge was already resolved")
		return fmt.Errorf("captcha challenge already resolved")
	}
}

func (cs *captchaServer) currentChallenge() *browserCaptchaChallenge {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.active == nil {
		return nil
	}

	challenge := cs.active.challenge
	return &challenge
}

func (cs *captchaServer) handlePage(w http.ResponseWriter, r *http.Request) {
	log := cs.requestLogger(r)
	if r.Method != http.MethodGet {
		log.Warn().Msg("Rejected CAPTCHA page request with unsupported method")
		writeCaptchaMethodNotAllowed(w, http.MethodGet)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(captchaPageHTML))
	log.Info().Msg("Served CAPTCHA page")
}

func (cs *captchaServer) handleChallenge(w http.ResponseWriter, r *http.Request) {
	log := cs.requestLogger(r)
	if r.Method != http.MethodGet {
		log.Warn().Msg("Rejected CAPTCHA challenge request with unsupported method")
		writeCaptchaMethodNotAllowed(w, http.MethodGet)
		return
	}

	challenge := cs.currentChallenge()
	if challenge == nil {
		log.Warn().Msg("Requested CAPTCHA challenge, but none is active")
		writeCaptchaJSON(w, http.StatusNotFound, captchaErrorResponse{Error: "no active captcha challenge"})
		return
	}

	log.Info().
		Str("challenge_id", challenge.ID).
		Str("captcha_service", string(challenge.Service)).
		Bool("captcha_invisible", challenge.Invisible).
		Bool("captcha_has_rqdata", challenge.RqData != "").
		Msg("Served active CAPTCHA challenge")
	writeCaptchaJSON(w, http.StatusOK, challenge)
}

func (cs *captchaServer) handleSolve(w http.ResponseWriter, r *http.Request) {
	log := cs.requestLogger(r)
	if r.Method != http.MethodPost {
		log.Warn().Msg("Rejected CAPTCHA solve request with unsupported method")
		writeCaptchaMethodNotAllowed(w, http.MethodPost)
		return
	}

	var req captchaSolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn().Err(err).Msg("Rejected CAPTCHA solve request with invalid JSON body")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		log.Warn().Msg("Rejected CAPTCHA solve request without challenge id")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "missing challenge id"})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		log.Warn().
			Str("challenge_id", req.ID).
			Msg("Rejected CAPTCHA solve request with empty token")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "missing captcha token"})
		return
	}

	if err := cs.resolveActiveChallenge(req.ID, captchaBrowserResult{token: req.Token}); err != nil {
		log.Warn().
			Str("challenge_id", req.ID).
			Err(err).
			Msg("Rejected CAPTCHA solve request")
		writeCaptchaJSON(w, http.StatusConflict, captchaErrorResponse{Error: err.Error()})
		return
	}

	log.Info().
		Str("challenge_id", req.ID).
		Int("token_length", len(req.Token)).
		Msg("Accepted CAPTCHA token from browser page")
	w.WriteHeader(http.StatusNoContent)
}

func (cs *captchaServer) handleCancel(w http.ResponseWriter, r *http.Request) {
	log := cs.requestLogger(r)
	if r.Method != http.MethodPost {
		log.Warn().Msg("Rejected CAPTCHA cancel request with unsupported method")
		writeCaptchaMethodNotAllowed(w, http.MethodPost)
		return
	}

	var req captchaCancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn().Err(err).Msg("Rejected CAPTCHA cancel request with invalid JSON body")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		log.Warn().Msg("Rejected CAPTCHA cancel request without challenge id")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "missing challenge id"})
		return
	}

	if err := cs.resolveActiveChallenge(req.ID, captchaBrowserResult{err: errCaptchaBrowserCanceled}); err != nil {
		log.Warn().
			Str("challenge_id", req.ID).
			Err(err).
			Msg("Rejected CAPTCHA cancel request")
		writeCaptchaJSON(w, http.StatusConflict, captchaErrorResponse{Error: err.Error()})
		return
	}

	log.Info().
		Str("challenge_id", req.ID).
		Msg("Browser page canceled CAPTCHA flow")
	w.WriteHeader(http.StatusNoContent)
}

func (cs *captchaServer) handleError(w http.ResponseWriter, r *http.Request) {
	log := cs.requestLogger(r)
	if r.Method != http.MethodPost {
		log.Warn().Msg("Rejected CAPTCHA error report with unsupported method")
		writeCaptchaMethodNotAllowed(w, http.MethodPost)
		return
	}

	var req captchaErrorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn().Err(err).Msg("Rejected CAPTCHA error report with invalid JSON body")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		log.Warn().Msg("Rejected CAPTCHA error report without challenge id")
		writeCaptchaJSON(w, http.StatusBadRequest, captchaErrorResponse{Error: "missing challenge id"})
		return
	}

	message := strings.TrimSpace(req.Error)
	if message == "" {
		message = "browser page reported an unknown error"
	}
	if err := cs.resolveActiveChallenge(req.ID, captchaBrowserResult{err: fmt.Errorf("%s", message)}); err != nil {
		log.Warn().
			Str("challenge_id", req.ID).
			Err(err).
			Msg("Rejected CAPTCHA browser error report")
		writeCaptchaJSON(w, http.StatusConflict, captchaErrorResponse{Error: err.Error()})
		return
	}

	log.Warn().
		Str("challenge_id", req.ID).
		Str("browser_error", message).
		Msg("Browser page reported CAPTCHA error")
	w.WriteHeader(http.StatusNoContent)
}

func (cs *captchaServer) requestLogger(r *http.Request) zerolog.Logger {
	return cs.log.With().
		Str("http_method", r.Method).
		Str("http_path", r.URL.Path).
		Str("remote_addr", r.RemoteAddr).
		Logger()
}

func writeCaptchaJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeCaptchaMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeCaptchaJSON(w, http.StatusMethodNotAllowed, captchaErrorResponse{Error: "method not allowed"})
}
