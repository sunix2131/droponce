export namespace application {
	
	export class CreateDirectTransferRequest {
	    sourcePath: string;
	    brokerUrl: string;
	    expiresInMinutes: number;
	    maxDownloads: number;
	    calculateHash: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateDirectTransferRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.brokerUrl = source["brokerUrl"];
	        this.expiresInMinutes = source["expiresInMinutes"];
	        this.maxDownloads = source["maxDownloads"];
	        this.calculateHash = source["calculateHash"];
	    }
	}
	export class CreateInternetTransferRequest {
	    sourcePath: string;
	    relayUrl: string;
	    recipientId: string;
	    expiresInMinutes: number;
	    maxDownloads: number;
	    calculateHash: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateInternetTransferRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.relayUrl = source["relayUrl"];
	        this.recipientId = source["recipientId"];
	        this.expiresInMinutes = source["expiresInMinutes"];
	        this.maxDownloads = source["maxDownloads"];
	        this.calculateHash = source["calculateHash"];
	    }
	}
	export class CreateTransferRequest {
	    sourcePath: string;
	    bindIp: string;
	    expiresInMinutes: number;
	    maxDownloads: number;
	    calculateHash: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateTransferRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.bindIp = source["bindIp"];
	        this.expiresInMinutes = source["expiresInMinutes"];
	        this.maxDownloads = source["maxDownloads"];
	        this.calculateHash = source["calculateHash"];
	    }
	}
	export class IncomingTransferDto {
	    sessionId: string;
	    status: string;
	    fileName?: string;
	    sizeBytes?: number;
	    bytesReceived: number;
	    savedPath?: string;
	    errorMessage?: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    completedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new IncomingTransferDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.status = source["status"];
	        this.fileName = source["fileName"];
	        this.sizeBytes = source["sizeBytes"];
	        this.bytesReceived = source["bytesReceived"];
	        this.savedPath = source["savedPath"];
	        this.errorMessage = source["errorMessage"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.completedAt = this.convertValues(source["completedAt"], null);
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
	export class TransferDetails {
	    id: string;
	    status: string;
	    sourceFileName: string;
	    sourcePath?: string;
	    sourceSizeBytes: number;
	    // Go type: time
	    sourceModifiedAt: any;
	    sourceSha256?: string;
	    bindIp?: string;
	    port?: number;
	    maxDownloads: number;
	    completedDownloads: number;
	    // Go type: time
	    expiresAt: any;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    activatedAt?: any;
	    // Go type: time
	    completedAt?: any;
	    // Go type: time
	    cancelledAt?: any;
	    // Go type: time
	    stoppedAt?: any;
	    lastErrorCode?: string;
	    lastErrorMessage?: string;
	    shareUrl?: string;
	    remainingDownloads: number;
	    qrCodePngBase64?: string;
	
	    static createFrom(source: any = {}) {
	        return new TransferDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.sourceFileName = source["sourceFileName"];
	        this.sourcePath = source["sourcePath"];
	        this.sourceSizeBytes = source["sourceSizeBytes"];
	        this.sourceModifiedAt = this.convertValues(source["sourceModifiedAt"], null);
	        this.sourceSha256 = source["sourceSha256"];
	        this.bindIp = source["bindIp"];
	        this.port = source["port"];
	        this.maxDownloads = source["maxDownloads"];
	        this.completedDownloads = source["completedDownloads"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.activatedAt = this.convertValues(source["activatedAt"], null);
	        this.completedAt = this.convertValues(source["completedAt"], null);
	        this.cancelledAt = this.convertValues(source["cancelledAt"], null);
	        this.stoppedAt = this.convertValues(source["stoppedAt"], null);
	        this.lastErrorCode = source["lastErrorCode"];
	        this.lastErrorMessage = source["lastErrorMessage"];
	        this.shareUrl = source["shareUrl"];
	        this.remainingDownloads = source["remainingDownloads"];
	        this.qrCodePngBase64 = source["qrCodePngBase64"];
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
	
	export class CreateCloudPubTransferRequest {
	    sourcePath: string;
	    bindIp: string;
	    expiresInMinutes: number;
	    maxDownloads: number;
	    calculateHash: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateCloudPubTransferRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.bindIp = source["bindIp"];
	        this.expiresInMinutes = source["expiresInMinutes"];
	        this.maxDownloads = source["maxDownloads"];
	        this.calculateHash = source["calculateHash"];
	    }
	}
	export class DiagnosticsDto {
	    version: string;
	    goVersion: string;
	    wailsVersion: string;
	    sqlitePath: string;
	    logsPath: string;
	    activeServerCount: number;
	    activeTransferCount: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticsDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.goVersion = source["goVersion"];
	        this.wailsVersion = source["wailsVersion"];
	        this.sqlitePath = source["sqlitePath"];
	        this.logsPath = source["logsPath"];
	        this.activeServerCount = source["activeServerCount"];
	        this.activeTransferCount = source["activeTransferCount"];
	    }
	}
	export class FileSelectionDto {
	    path: string;
	    name: string;
	    sizeBytes: number;
	    // Go type: time
	    modifiedAt: any;
	    isSymlink: boolean;
	    symlinkWarning?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileSelectionDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	        this.modifiedAt = this.convertValues(source["modifiedAt"], null);
	        this.isSymlink = source["isSymlink"];
	        this.symlinkWarning = source["symlinkWarning"];
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
	export class QrCodeDto {
	    pngBase64: string;
	
	    static createFrom(source: any = {}) {
	        return new QrCodeDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pngBase64 = source["pngBase64"];
	    }
	}
	export class RelayRecommendationDto {
	    url: string;
	    isLocalLan: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RelayRecommendationDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.isLocalLan = source["isLocalLan"];
	    }
	}
	export class SaveResultDto {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveResultDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class SettingsDto {
	    language: string;
	    theme: string;
	    defaultRelayUrl: string;
	    defaultExpiryMinutes: number;
	    defaultMaxDownloads: number;
	    defaultCalculateSha: boolean;
	    warnTrustedLocalOnly: boolean;
	    maxActiveTransfers: number;
	    confirmCloseWithLinks: boolean;
	    cloudPubToken: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.theme = source["theme"];
	        this.defaultRelayUrl = source["defaultRelayUrl"];
	        this.defaultExpiryMinutes = source["defaultExpiryMinutes"];
	        this.defaultMaxDownloads = source["defaultMaxDownloads"];
	        this.defaultCalculateSha = source["defaultCalculateSha"];
	        this.warnTrustedLocalOnly = source["warnTrustedLocalOnly"];
	        this.maxActiveTransfers = source["maxActiveTransfers"];
	        this.confirmCloseWithLinks = source["confirmCloseWithLinks"];
	        this.cloudPubToken = source["cloudPubToken"];
	    }
	}
	export class TransferLimitsDto {
	    localSingleFileLimitLabel: string;
	    internetSingleFileLimitGb: number;
	    internetSingleFileLimitText: string;
	    multiFileSupported: boolean;
	    multiFileAdvice: string;
	
	    static createFrom(source: any = {}) {
	        return new TransferLimitsDto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localSingleFileLimitLabel = source["localSingleFileLimitLabel"];
	        this.internetSingleFileLimitGb = source["internetSingleFileLimitGb"];
	        this.internetSingleFileLimitText = source["internetSingleFileLimitText"];
	        this.multiFileSupported = source["multiFileSupported"];
	        this.multiFileAdvice = source["multiFileAdvice"];
	    }
	}

}

