export namespace desktop {
	
	export class Diff {
	    component: string;
	    action: string;
	    target: string;
	    current: string;
	    proposed: string;
	    risk: string;
	
	    static createFrom(source: any = {}) {
	        return new Diff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.component = source["component"];
	        this.action = source["action"];
	        this.target = source["target"];
	        this.current = source["current"];
	        this.proposed = source["proposed"];
	        this.risk = source["risk"];
	    }
	}
	export class HealthReport {
	    database: string;
	    role: string;
	    oplog_window: string;
	    oplog_size: string;
	    connections: string;
	    lag?: Record<string, string>;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new HealthReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.role = source["role"];
	        this.oplog_window = source["oplog_window"];
	        this.oplog_size = source["oplog_size"];
	        this.connections = source["connections"];
	        this.lag = source["lag"];
	        this.warnings = source["warnings"];
	    }
	}
	export class IndexSpec {
	    collection: string;
	    name: string;
	    keys: string;
	    unique: boolean;
	    sparse: boolean;
	    partial_filter?: string;
	    expire_after_seconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new IndexSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collection = source["collection"];
	        this.name = source["name"];
	        this.keys = source["keys"];
	        this.unique = source["unique"];
	        this.sparse = source["sparse"];
	        this.partial_filter = source["partial_filter"];
	        this.expire_after_seconds = source["expire_after_seconds"];
	    }
	}
	export class MigrationRecord {
	    version: string;
	    description: string;
	    // Go type: time
	    applied_at: any;
	    checksum: string;
	
	    static createFrom(source: any = {}) {
	        return new MigrationRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.description = source["description"];
	        this.applied_at = this.convertValues(source["applied_at"], null);
	        this.checksum = source["checksum"];
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
	export class MigrationStatus {
	    version: string;
	    description: string;
	    applied: boolean;
	    // Go type: time
	    applied_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new MigrationStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.description = source["description"];
	        this.applied = source["applied"];
	        this.applied_at = this.convertValues(source["applied_at"], null);
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

}

export namespace main {
	
	export class SavedConnection {
	    name: string;
	    url: string;
	    database: string;
	    username: string;
	    password: string;
	    last_used: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.database = source["database"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.last_used = source["last_used"];
	    }
	}

}

