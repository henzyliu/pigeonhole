export namespace gifs {
	
	export class GIF {
	    id: string;
	    title: string;
	    preview: string;
	    previewWidth: number;
	    previewHeight: number;
	
	    static createFrom(source: any = {}) {
	        return new GIF(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.preview = source["preview"];
	        this.previewWidth = source["previewWidth"];
	        this.previewHeight = source["previewHeight"];
	    }
	}

}

export namespace inbox {
	
	export class Account {
	    network: string;
	    networkName: string;
	    id: string;
	    name: string;
	    state: string;
	    error: string;
	    needsLogin: boolean;
	    historyPending: number;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = source["network"];
	        this.networkName = source["networkName"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.needsLogin = source["needsLogin"];
	        this.historyPending = source["historyPending"];
	    }
	}
	export class Reader {
	    name: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new Reader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	    }
	}
	export class Chat {
	    roomId: string;
	    network: string;
	    name: string;
	    avatar: string;
	    preview: string;
	    lastTs: number;
	    unread: number;
	    isDm: boolean;
	    category: string;
	    hidden: boolean;
	    lastFromMe: boolean;
	    lastStatus: string;
	    seenBy: Reader[];
	    historyLoading: boolean;
	    muted: boolean;
	    mutedOnNetwork: boolean;
	    draft: string;
	    draftReply: string;
	
	    static createFrom(source: any = {}) {
	        return new Chat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roomId = source["roomId"];
	        this.network = source["network"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.preview = source["preview"];
	        this.lastTs = source["lastTs"];
	        this.unread = source["unread"];
	        this.isDm = source["isDm"];
	        this.category = source["category"];
	        this.hidden = source["hidden"];
	        this.lastFromMe = source["lastFromMe"];
	        this.lastStatus = source["lastStatus"];
	        this.seenBy = this.convertValues(source["seenBy"], Reader);
	        this.historyLoading = source["historyLoading"];
	        this.muted = source["muted"];
	        this.mutedOnNetwork = source["mutedOnNetwork"];
	        this.draft = source["draft"];
	        this.draftReply = source["draftReply"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkPreview {
	    url: string;
	    site: string;
	    title: string;
	    description: string;
	    image: string;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new LinkPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.site = source["site"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.image = source["image"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class LoginField {
	    id: string;
	    name: string;
	    type: string;
	    description: string;
	    default: string;
	    optional: boolean;
	    options: string[];
	
	    static createFrom(source: any = {}) {
	        return new LoginField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.default = source["default"];
	        this.optional = source["optional"];
	        this.options = source["options"];
	    }
	}
	export class LoginFlow {
	    id: string;
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginFlow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class LoginStep {
	    sessionId: string;
	    network: string;
	    type: string;
	    instructions: string;
	    fields: LoginField[];
	    cookieUrl: string;
	    cookieNames: string[];
	    needsBrowser: boolean;
	    browserAvailable: boolean;
	    displayType: string;
	    displayData: string;
	    qrImage: string;
	    accountId: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.network = source["network"];
	        this.type = source["type"];
	        this.instructions = source["instructions"];
	        this.fields = this.convertValues(source["fields"], LoginField);
	        this.cookieUrl = source["cookieUrl"];
	        this.cookieNames = source["cookieNames"];
	        this.needsBrowser = source["needsBrowser"];
	        this.browserAvailable = source["browserAvailable"];
	        this.displayType = source["displayType"];
	        this.displayData = source["displayData"];
	        this.qrImage = source["qrImage"];
	        this.accountId = source["accountId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Member {
	    userId: string;
	    name: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new Member(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	    }
	}
	export class Mention {
	    userId: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Mention(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.name = source["name"];
	    }
	}
	export class Reaction {
	    key: string;
	    count: number;
	    mine: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Reaction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.count = source["count"];
	        this.mine = source["mine"];
	    }
	}
	export class Message {
	    eventId: string;
	    senderName: string;
	    senderAvatar: string;
	    isMe: boolean;
	    ts: number;
	    kind: string;
	    body: string;
	    mediaUrl: string;
	    mimeType: string;
	    fileName: string;
	    width: number;
	    height: number;
	    thumbnail: string;
	    status: string;
	    edited: boolean;
	    redacted: boolean;
	    replyTo: string;
	    reactions: Reaction[];
	    seenBy: Reader[];
	    fetchable: string;
	    previews: LinkPreview[];
	    pending: string;
	    changeFailed: string;
	    canEdit: boolean;
	    canUnsend: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.eventId = source["eventId"];
	        this.senderName = source["senderName"];
	        this.senderAvatar = source["senderAvatar"];
	        this.isMe = source["isMe"];
	        this.ts = source["ts"];
	        this.kind = source["kind"];
	        this.body = source["body"];
	        this.mediaUrl = source["mediaUrl"];
	        this.mimeType = source["mimeType"];
	        this.fileName = source["fileName"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.thumbnail = source["thumbnail"];
	        this.status = source["status"];
	        this.edited = source["edited"];
	        this.redacted = source["redacted"];
	        this.replyTo = source["replyTo"];
	        this.reactions = this.convertValues(source["reactions"], Reaction);
	        this.seenBy = this.convertValues(source["seenBy"], Reader);
	        this.fetchable = source["fetchable"];
	        this.previews = this.convertValues(source["previews"], LinkPreview);
	        this.pending = source["pending"];
	        this.changeFailed = source["changeFailed"];
	        this.canEdit = source["canEdit"];
	        this.canUnsend = source["canUnsend"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Person {
	    network: string;
	    networkName: string;
	    loginId: string;
	    id: string;
	    name: string;
	    avatar: string;
	    roomId: string;
	
	    static createFrom(source: any = {}) {
	        return new Person(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = source["network"];
	        this.networkName = source["networkName"];
	        this.loginId = source["loginId"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.roomId = source["roomId"];
	    }
	}
	
	
	export class SearchResult {
	    roomId: string;
	    eventId: string;
	    chatName: string;
	    chatAvatar: string;
	    network: string;
	    isDm: boolean;
	    senderName: string;
	    isMe: boolean;
	    ts: number;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roomId = source["roomId"];
	        this.eventId = source["eventId"];
	        this.chatName = source["chatName"];
	        this.chatAvatar = source["chatAvatar"];
	        this.network = source["network"];
	        this.isDm = source["isDm"];
	        this.senderName = source["senderName"];
	        this.isMe = source["isMe"];
	        this.ts = source["ts"];
	        this.body = source["body"];
	    }
	}

}

export namespace main {
	
	export class NotificationPermission {
	    status: string;
	    system: string;
	
	    static createFrom(source: any = {}) {
	        return new NotificationPermission(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.system = source["system"];
	    }
	}
	export class Settings {
	    theme: string;
	    notificationsOff: boolean;
	    hideMessageText: boolean;
	    mutedAccounts?: string[];
	    updateNotified?: string;
	    backgroundNoticeShown: boolean;
	    welcomeDone: boolean;
	    singleChatList: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.notificationsOff = source["notificationsOff"];
	        this.hideMessageText = source["hideMessageText"];
	        this.mutedAccounts = source["mutedAccounts"];
	        this.updateNotified = source["updateNotified"];
	        this.backgroundNoticeShown = source["backgroundNoticeShown"];
	        this.welcomeDone = source["welcomeDone"];
	        this.singleChatList = source["singleChatList"];
	    }
	}
	export class StartAtLogin {
	    supported: boolean;
	    enabled: boolean;
	    system: string;
	
	    static createFrom(source: any = {}) {
	        return new StartAtLogin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.enabled = source["enabled"];
	        this.system = source["system"];
	    }
	}
	export class UpdateStatus {
	    state: string;
	    current: string;
	    latest?: update.Release;
	    progress: number;
	    error: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.current = source["current"];
	        this.latest = this.convertValues(source["latest"], update.Release);
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.canInstall = source["canInstall"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VersionInfo {
	    version: string;
	    commit: string;
	    branch: string;
	    buildDate: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.branch = source["branch"];
	        this.buildDate = source["buildDate"];
	    }
	}

}

export namespace update {
	
	export class Release {
	    version: string;
	    pageUrl: string;
	    notes: string;
	    asset: string;
	    assetUrl: string;
	    assetSize: number;
	    checksumsUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.pageUrl = source["pageUrl"];
	        this.notes = source["notes"];
	        this.asset = source["asset"];
	        this.assetUrl = source["assetUrl"];
	        this.assetSize = source["assetSize"];
	        this.checksumsUrl = source["checksumsUrl"];
	    }
	}

}

