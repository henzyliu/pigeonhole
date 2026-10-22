// Package inbox runs the mautrix network bridges in-process on top of the
// localmx store and exposes a small API for the desktop UI.
package inbox

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-meta/pkg/connector"
	"go.mau.fi/mautrix-meta/pkg/igconnector"
	"go.mau.fi/util/dbutil"
	_ "go.mau.fi/util/dbutil/litestream"
	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/bridgeconfig"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

// Me is the local user's ID inside the in-process "homeserver".
var Me = id.NewUserID("me", localmx.ServerName)

type Network struct {
	ID     string
	Name   string
	Bridge *bridgev2.Bridge
	MX     *localmx.Connector
}

type Service struct {
	Store *localmx.Store
	Log   zerolog.Logger
	// Emit pushes an event to the UI.
	Emit func(name string, data any)

	ctx      context.Context
	networks map[string]*Network
	order    []string

	lock   sync.Mutex
	logins map[string]*pendingLogin
	states map[string]status.BridgeState
}

func New(dataDir string, log zerolog.Logger) (*Service, error) {
	store, err := localmx.OpenStore(dataDir, Me)
	if err != nil {
		return nil, fmt.Errorf("failed to open inbox store: %w", err)
	}
	s := &Service{
		Store:    store,
		Log:      log,
		networks: make(map[string]*Network),
		logins:   make(map[string]*pendingLogin),
		states:   make(map[string]status.BridgeState),
	}
	store.IsSelf = s.isSelf
	store.Notify = func(change localmx.Change) { s.emit("inbox:change", change) }

	fb := &connector.MetaConnector{}
	if err = loadConfig(connector.ExampleConfig, &fb.Config); err != nil {
		return nil, err
	}
	ig := &igconnector.IGConnector{}
	if err = loadConfig(igconnector.ExampleConfig, &ig.Config); err != nil {
		return nil, err
	}
	if err = s.addNetwork(dataDir, "messenger", "Messenger", fb); err != nil {
		return nil, err
	}
	if err = s.addNetwork(dataDir, "instagram", "Instagram", ig); err != nil {
		return nil, err
	}
	return s, nil
}

// loadConfig fills a connector config with the defaults from its example YAML.
func loadConfig(example string, into any) error {
	return yaml.Unmarshal([]byte(example), into)
}

func bridgeConfig() *bridgeconfig.BridgeConfig {
	return &bridgeconfig.BridgeConfig{
		CommandPrefix:         "!inbox",
		PrivateChatPortalMeta: true,
		Permissions: bridgeconfig.PermissionConfig{
			"*": &bridgeconfig.Permissions{SendEvents: true, Commands: true, Login: true, DoublePuppet: true, Admin: true},
		},
		Backfill: bridgeconfig.BackfillConfig{
			Enabled:              true,
			MaxInitialMessages:   50,
			MaxCatchupMessages:   500,
			UnreadHoursThreshold: 720,
		},
	}
}

func (s *Service) addNetwork(dataDir, netID, name string, network bridgev2.NetworkConnector) error {
	log := s.Log.With().Str("network", netID).Logger()
	db, err := dbutil.NewFromConfig("unified-inbox-"+netID, dbutil.Config{
		PoolConfig: dbutil.PoolConfig{
			Type:         "sqlite3-fk-wal",
			URI:          "file:" + filepath.Join(dataDir, netID+".db") + "?_txlock=immediate",
			MaxOpenConns: 5,
			MaxIdleConns: 1,
		},
	}, dbutil.ZeroLogger(log.With().Str("db_section", "main").Logger()))
	if err != nil {
		return fmt.Errorf("failed to open %s database: %w", netID, err)
	}
	mx := localmx.NewConnector(s.Store, netID)
	mx.OnBridgeState = s.onBridgeState
	br := bridgev2.NewBridge("", db, log, bridgeConfig(), mx, network, commands.NewProcessor)
	s.networks[netID] = &Network{ID: netID, Name: name, Bridge: br, MX: mx}
	s.order = append(s.order, netID)
	return nil
}

// Start connects every saved account. It blocks until all bridges are up.
func (s *Service) Start(ctx context.Context) error {
	s.ctx = ctx
	for _, netID := range s.order {
		n := s.networks[netID]
		if err := n.Bridge.Start(ctx); err != nil {
			return fmt.Errorf("failed to start %s: %w", n.Name, err)
		}
		if _, err := n.Bridge.GetUserByMXID(ctx, Me); err != nil {
			return fmt.Errorf("failed to create %s user: %w", n.Name, err)
		}
	}
	s.emit("inbox:accounts", nil)
	return nil
}

func (s *Service) Stop() {
	for _, n := range s.networks {
		n.Bridge.Stop()
	}
	_ = s.Store.DB.Close()
}

func (s *Service) emit(name string, data any) {
	if s.Emit != nil {
		s.Emit(name, data)
	}
}

// isSelf reports whether a ghost belongs to one of the local user's own
// accounts (Meta uses the same ID for the login and the user's ghost).
func (s *Service) isSelf(userID id.UserID) bool {
	for _, n := range s.networks {
		if ghostID, ok := n.MX.ParseGhostMXID(userID); ok {
			return n.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(ghostID)) != nil
		}
	}
	return false
}

func (s *Service) onBridgeState(network string, state status.BridgeState) {
	if state.RemoteID == "" {
		return
	}
	s.lock.Lock()
	s.states[network+"/"+string(state.RemoteID)] = state
	s.lock.Unlock()
	s.emit("inbox:accounts", nil)
}

func (s *Service) network(netID string) (*Network, error) {
	n, ok := s.networks[netID]
	if !ok {
		return nil, fmt.Errorf("unknown network %q", netID)
	}
	return n, nil
}
