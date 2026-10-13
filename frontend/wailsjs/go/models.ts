export namespace main {
	
	export class DesktopSceneLink {
	    route: string;
	    error?: string;

	    static createFrom(source: any = {}) {
	        return new DesktopSceneLink(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.route = source["route"];
	        this.error = source["error"];
	    }
	}
	export class DesktopUpdateStatus {
	    checked: boolean;
	    current: string;
	    latest: string;
	    updateAvailable: boolean;
	    releaseUrl: string;
	    assetName: string;
	    cached: boolean;
	    supported: boolean;
	    canInstall: boolean;
	    installStarted: boolean;
	    restartRequired: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new DesktopUpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checked = source["checked"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.updateAvailable = source["updateAvailable"];
	        this.releaseUrl = source["releaseUrl"];
	        this.assetName = source["assetName"];
	        this.cached = source["cached"];
	        this.supported = source["supported"];
	        this.canInstall = source["canInstall"];
	        this.installStarted = source["installStarted"];
	        this.restartRequired = source["restartRequired"];
	        this.message = source["message"];
	    }
	}
	export class DialogFilter {
	    displayName: string;
	    pattern: string;
	
	    static createFrom(source: any = {}) {
	        return new DialogFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayName = source["displayName"];
	        this.pattern = source["pattern"];
	    }
	}
	export class FileDialogResult {
	    path: string;
	    content: string;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileDialogResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.canceled = source["canceled"];
	    }
	}
	export class SaveFileResult {
	    path: string;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SaveFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.canceled = source["canceled"];
	    }
	}

}

