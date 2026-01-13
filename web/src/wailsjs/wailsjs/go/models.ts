export namespace desktop {
	
	export class AntigravityTokenValidationResult {
	    valid: boolean;
	    email?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new AntigravityTokenValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.email = source["email"];
	        this.error = source["error"];
	    }
	}
	export class AntigravityBatchValidationResult {
	    results: AntigravityTokenValidationResult[];
	
	    static createFrom(source: any = {}) {
	        return new AntigravityBatchValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], AntigravityTokenValidationResult);
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
	export class AntigravityQuotaData {
	    email: string;
	    name: string;
	    picture: string;
	    projectId: string;
	    subscriptionTier: string;
	    isForbidden: boolean;
	    models: domain.AntigravityModelQuota[];
	    lastUpdated: number;
	
	    static createFrom(source: any = {}) {
	        return new AntigravityQuotaData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.name = source["name"];
	        this.picture = source["picture"];
	        this.projectId = source["projectId"];
	        this.subscriptionTier = source["subscriptionTier"];
	        this.isForbidden = source["isForbidden"];
	        this.models = this.convertValues(source["models"], domain.AntigravityModelQuota);
	        this.lastUpdated = source["lastUpdated"];
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

export namespace domain {
	
	export class AntigravityModelQuota {
	    name: string;
	    percentage: number;
	    resetTime: string;
	
	    static createFrom(source: any = {}) {
	        return new AntigravityModelQuota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.percentage = source["percentage"];
	        this.resetTime = source["resetTime"];
	    }
	}
	export class Cooldown {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    providerID: number;
	    clientType: string;
	    // Go type: time
	    untilTime: any;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Cooldown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.providerID = source["providerID"];
	        this.clientType = source["clientType"];
	        this.untilTime = this.convertValues(source["untilTime"], null);
	        this.reason = source["reason"];
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
	export class Project {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    name: string;
	    slug: string;
	    enabledCustomRoutes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.name = source["name"];
	        this.slug = source["slug"];
	        this.enabledCustomRoutes = source["enabledCustomRoutes"];
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
	export class ProviderConfigAntigravity {
	    email: string;
	    refreshToken: string;
	    projectID: string;
	    endpoint: string;
	    modelMapping?: Record<string, string>;
	    haikuTarget?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfigAntigravity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.refreshToken = source["refreshToken"];
	        this.projectID = source["projectID"];
	        this.endpoint = source["endpoint"];
	        this.modelMapping = source["modelMapping"];
	        this.haikuTarget = source["haikuTarget"];
	    }
	}
	export class ProviderConfigCustom {
	    baseURL: string;
	    apiKey: string;
	    clientBaseURL?: Record<string, string>;
	    modelMapping?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfigCustom(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseURL = source["baseURL"];
	        this.apiKey = source["apiKey"];
	        this.clientBaseURL = source["clientBaseURL"];
	        this.modelMapping = source["modelMapping"];
	    }
	}
	export class ProviderConfig {
	    custom?: ProviderConfigCustom;
	    antigravity?: ProviderConfigAntigravity;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.custom = this.convertValues(source["custom"], ProviderConfigCustom);
	        this.antigravity = this.convertValues(source["antigravity"], ProviderConfigAntigravity);
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
	export class Provider {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    // Go type: time
	    deletedAt?: any;
	    type: string;
	    name: string;
	    config?: ProviderConfig;
	    supportedClientTypes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.deletedAt = this.convertValues(source["deletedAt"], null);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.config = this.convertValues(source["config"], ProviderConfig);
	        this.supportedClientTypes = source["supportedClientTypes"];
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
	
	
	
	export class ResponseInfo {
	    status: number;
	    headers: Record<string, string>;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new ResponseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.headers = source["headers"];
	        this.body = source["body"];
	    }
	}
	export class RequestInfo {
	    method: string;
	    headers: Record<string, string>;
	    url: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new RequestInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.method = source["method"];
	        this.headers = source["headers"];
	        this.url = source["url"];
	        this.body = source["body"];
	    }
	}
	export class ProxyRequest {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    instanceID: string;
	    requestID: string;
	    sessionID: string;
	    clientType: string;
	    requestModel: string;
	    responseModel: string;
	    // Go type: time
	    startTime: any;
	    // Go type: time
	    endTime: any;
	    duration: number;
	    isStream: boolean;
	    status: string;
	    statusCode: number;
	    requestInfo?: RequestInfo;
	    responseInfo?: ResponseInfo;
	    error: string;
	    proxyUpstreamAttemptCount: number;
	    finalProxyUpstreamAttemptID: number;
	    routeID: number;
	    providerID: number;
	    projectID: number;
	    inputTokenCount: number;
	    outputTokenCount: number;
	    cacheReadCount: number;
	    cacheWriteCount: number;
	    cache5mWriteCount: number;
	    cache1hWriteCount: number;
	    cost: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.instanceID = source["instanceID"];
	        this.requestID = source["requestID"];
	        this.sessionID = source["sessionID"];
	        this.clientType = source["clientType"];
	        this.requestModel = source["requestModel"];
	        this.responseModel = source["responseModel"];
	        this.startTime = this.convertValues(source["startTime"], null);
	        this.endTime = this.convertValues(source["endTime"], null);
	        this.duration = source["duration"];
	        this.isStream = source["isStream"];
	        this.status = source["status"];
	        this.statusCode = source["statusCode"];
	        this.requestInfo = this.convertValues(source["requestInfo"], RequestInfo);
	        this.responseInfo = this.convertValues(source["responseInfo"], ResponseInfo);
	        this.error = source["error"];
	        this.proxyUpstreamAttemptCount = source["proxyUpstreamAttemptCount"];
	        this.finalProxyUpstreamAttemptID = source["finalProxyUpstreamAttemptID"];
	        this.routeID = source["routeID"];
	        this.providerID = source["providerID"];
	        this.projectID = source["projectID"];
	        this.inputTokenCount = source["inputTokenCount"];
	        this.outputTokenCount = source["outputTokenCount"];
	        this.cacheReadCount = source["cacheReadCount"];
	        this.cacheWriteCount = source["cacheWriteCount"];
	        this.cache5mWriteCount = source["cache5mWriteCount"];
	        this.cache1hWriteCount = source["cache1hWriteCount"];
	        this.cost = source["cost"];
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
	export class ProxyUpstreamAttempt {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    // Go type: time
	    startTime: any;
	    // Go type: time
	    endTime: any;
	    duration: number;
	    status: string;
	    proxyRequestID: number;
	    isStream: boolean;
	    requestInfo?: RequestInfo;
	    responseInfo?: ResponseInfo;
	    routeID: number;
	    providerID: number;
	    inputTokenCount: number;
	    outputTokenCount: number;
	    cacheReadCount: number;
	    cacheWriteCount: number;
	    cache5mWriteCount: number;
	    cache1hWriteCount: number;
	    cost: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyUpstreamAttempt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.startTime = this.convertValues(source["startTime"], null);
	        this.endTime = this.convertValues(source["endTime"], null);
	        this.duration = source["duration"];
	        this.status = source["status"];
	        this.proxyRequestID = source["proxyRequestID"];
	        this.isStream = source["isStream"];
	        this.requestInfo = this.convertValues(source["requestInfo"], RequestInfo);
	        this.responseInfo = this.convertValues(source["responseInfo"], ResponseInfo);
	        this.routeID = source["routeID"];
	        this.providerID = source["providerID"];
	        this.inputTokenCount = source["inputTokenCount"];
	        this.outputTokenCount = source["outputTokenCount"];
	        this.cacheReadCount = source["cacheReadCount"];
	        this.cacheWriteCount = source["cacheWriteCount"];
	        this.cache5mWriteCount = source["cache5mWriteCount"];
	        this.cache1hWriteCount = source["cache1hWriteCount"];
	        this.cost = source["cost"];
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
	
	
	export class RetryConfig {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    name: string;
	    isDefault: boolean;
	    maxRetries: number;
	    initialInterval: number;
	    backoffRate: number;
	    maxInterval: number;
	
	    static createFrom(source: any = {}) {
	        return new RetryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.name = source["name"];
	        this.isDefault = source["isDefault"];
	        this.maxRetries = source["maxRetries"];
	        this.initialInterval = source["initialInterval"];
	        this.backoffRate = source["backoffRate"];
	        this.maxInterval = source["maxInterval"];
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
	export class Route {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    isEnabled: boolean;
	    isNative: boolean;
	    projectID: number;
	    clientType: string;
	    providerID: number;
	    position: number;
	    retryConfigID: number;
	    modelMapping?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Route(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.isEnabled = source["isEnabled"];
	        this.isNative = source["isNative"];
	        this.projectID = source["projectID"];
	        this.clientType = source["clientType"];
	        this.providerID = source["providerID"];
	        this.position = source["position"];
	        this.retryConfigID = source["retryConfigID"];
	        this.modelMapping = source["modelMapping"];
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
	export class RoutingStrategyConfig {
	
	
	    static createFrom(source: any = {}) {
	        return new RoutingStrategyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class RoutingStrategy {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    projectID: number;
	    type: string;
	    // Go type: RoutingStrategyConfig
	    config?: any;
	
	    static createFrom(source: any = {}) {
	        return new RoutingStrategy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.projectID = source["projectID"];
	        this.type = source["type"];
	        this.config = this.convertValues(source["config"], null);
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
	export class Session {
	    id: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    sessionID: string;
	    clientType: string;
	    projectID: number;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.sessionID = source["sessionID"];
	        this.clientType = source["clientType"];
	        this.projectID = source["projectID"];
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

export namespace service {
	
	export class CursorPaginationResult {
	    items: domain.ProxyRequest[];
	    hasMore: boolean;
	    firstId?: number;
	    lastId?: number;
	
	    static createFrom(source: any = {}) {
	        return new CursorPaginationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], domain.ProxyRequest);
	        this.hasMore = source["hasMore"];
	        this.firstId = source["firstId"];
	        this.lastId = source["lastId"];
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
	export class ImportResult {
	    imported: number;
	    skipped: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	}
	export class LogsResult {
	    lines: string[];
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new LogsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lines = source["lines"];
	        this.count = source["count"];
	    }
	}
	export class ProxyStatus {
	    running: boolean;
	    address: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.address = source["address"];
	        this.port = source["port"];
	    }
	}
	export class UpdateSessionProjectResult {
	    session?: domain.Session;
	    updatedRequests: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateSessionProjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = this.convertValues(source["session"], domain.Session);
	        this.updatedRequests = source["updatedRequests"];
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

