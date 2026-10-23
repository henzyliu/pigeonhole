export namespace inbox {
	
	export class Account {
	    network: string;
	    networkName: string;
	    id: string;
	    name: string;
	    state: string;
	    error: string;
	
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
	    status: string;
	    edited: boolean;
	    redacted: boolean;
	    replyTo: string;
	    reactions: Reaction[];
	    seenBy: Reader[];
	
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
	        this.status = source["status"];
	        this.edited = source["edited"];
	        this.redacted = source["redacted"];
	        this.replyTo = source["replyTo"];
	        this.reactions = this.convertValues(source["reactions"], Reaction);
	        this.seenBy = this.convertValues(source["seenBy"], Reader);
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
	

}

