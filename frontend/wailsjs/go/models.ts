export namespace model {
	
	export class AppSettings {
	    gpuDisabled: boolean;
	    defaultShell: string;
	    gitBashPath: string;
	    wslDistro: string;
	    searchExcludeDirs: string[];
	    searchExcludeFiles: string[];
	    shortcutCommandPalette: string;
	    shortcutToggleTerminal: string;
	    obsidianPath: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gpuDisabled = source["gpuDisabled"];
	        this.defaultShell = source["defaultShell"];
	        this.gitBashPath = source["gitBashPath"];
	        this.wslDistro = source["wslDistro"];
	        this.searchExcludeDirs = source["searchExcludeDirs"];
	        this.searchExcludeFiles = source["searchExcludeFiles"];
	        this.shortcutCommandPalette = source["shortcutCommandPalette"];
	        this.shortcutToggleTerminal = source["shortcutToggleTerminal"];
	        this.obsidianPath = source["obsidianPath"];
	    }
	}
	export class BranchInfo {
	    name: string;
	    isRemote: boolean;
	    isCurrent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BranchInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isRemote = source["isRemote"];
	        this.isCurrent = source["isCurrent"];
	    }
	}
	export class BranchList {
	    branches: BranchInfo[];
	
	    static createFrom(source: any = {}) {
	        return new BranchList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.branches = this.convertValues(source["branches"], BranchInfo);
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
	export class Commit {
	    sha: string;
	    shortSha: string;
	    message: string;
	    author: string;
	    email: string;
	    timestamp: number;
	    dateTime: string;
	    files: string[];
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.shortSha = source["shortSha"];
	        this.message = source["message"];
	        this.author = source["author"];
	        this.email = source["email"];
	        this.timestamp = source["timestamp"];
	        this.dateTime = source["dateTime"];
	        this.files = source["files"];
	    }
	}
	export class ContentSearchResult {
	    repoName: string;
	    repoPath: string;
	    filePath: string;
	    lineNum: number;
	    lineText: string;
	
	    static createFrom(source: any = {}) {
	        return new ContentSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoName = source["repoName"];
	        this.repoPath = source["repoPath"];
	        this.filePath = source["filePath"];
	        this.lineNum = source["lineNum"];
	        this.lineText = source["lineText"];
	    }
	}
	export class ContentSearchGroup {
	    repoName: string;
	    repoPath: string;
	    items: ContentSearchResult[];
	
	    static createFrom(source: any = {}) {
	        return new ContentSearchGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoName = source["repoName"];
	        this.repoPath = source["repoPath"];
	        this.items = this.convertValues(source["items"], ContentSearchResult);
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
	
	export class Directory {
	    id: string;
	    name: string;
	    path: string;
	    isDefault: boolean;
	    // Go type: time
	    createTime: any;
	
	    static createFrom(source: any = {}) {
	        return new Directory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDefault = source["isDefault"];
	        this.createTime = this.convertValues(source["createTime"], null);
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
	export class Favorite {
	    path: string;
	    alias?: string;
	    group: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Favorite(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.group = source["group"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class FileBytes {
	    path: string;
	    name: string;
	    size: number;
	    kind?: string;
	    base64?: string;
	    tooLarge: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileBytes(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.kind = source["kind"];
	        this.base64 = source["base64"];
	        this.tooLarge = source["tooLarge"];
	        this.error = source["error"];
	    }
	}
	export class FileChange {
	    path: string;
	    status: string;
	    staged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.staged = source["staged"];
	    }
	}
	export class FilePreview {
	    path: string;
	    name: string;
	    size: number;
	    content?: string;
	    isBinary: boolean;
	    tooLarge: boolean;
	    error?: string;
	    kind?: string;
	
	    static createFrom(source: any = {}) {
	        return new FilePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.content = source["content"];
	        this.isBinary = source["isBinary"];
	        this.tooLarge = source["tooLarge"];
	        this.error = source["error"];
	        this.kind = source["kind"];
	    }
	}
	export class FileTreeNode {
	    id: string;
	    name: string;
	    path: string;
	    type: string;
	    isGitRepo: boolean;
	    hasChildren: boolean;
	    children?: FileTreeNode[];
	    isLeaf: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileTreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.type = source["type"];
	        this.isGitRepo = source["isGitRepo"];
	        this.hasChildren = source["hasChildren"];
	        this.children = this.convertValues(source["children"], FileTreeNode);
	        this.isLeaf = source["isLeaf"];
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
	export class GitCommit {
	    hash: string;
	    author: string;
	    // Go type: time
	    date: any;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.author = source["author"];
	        this.date = this.convertValues(source["date"], null);
	        this.message = source["message"];
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
	export class GitRemoteInfo {
	    remoteUrl: string;
	    branch: string;
	    isDetached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitRemoteInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.remoteUrl = source["remoteUrl"];
	        this.branch = source["branch"];
	        this.isDetached = source["isDetached"];
	    }
	}
	export class GitRepoInfo {
	    path: string;
	    branch: string;
	    remote: string;
	    remoteUrl: string;
	    commits: GitCommit[];
	    isRepo: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitRepoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.branch = source["branch"];
	        this.remote = source["remote"];
	        this.remoteUrl = source["remoteUrl"];
	        this.commits = this.convertValues(source["commits"], GitCommit);
	        this.isRepo = source["isRepo"];
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
	export class PageResult {
	    records: any;
	    total: number;
	    current: number;
	    size: number;
	    pages: number;
	
	    static createFrom(source: any = {}) {
	        return new PageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = source["records"];
	        this.total = source["total"];
	        this.current = source["current"];
	        this.size = source["size"];
	        this.pages = source["pages"];
	    }
	}
	export class PullSummary {
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PullSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	    }
	}
	export class SearchResult {
	    name: string;
	    path: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.type = source["type"];
	    }
	}
	export class ShellConfig {
	    type: string;
	    executable: string;
	    args?: string[];
	    displayName: string;
	
	    static createFrom(source: any = {}) {
	        return new ShellConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.executable = source["executable"];
	        this.args = source["args"];
	        this.displayName = source["displayName"];
	    }
	}
	export class UpdateInfo {
	    hasUpdate: boolean;
	    currentVer: string;
	    latestVer: string;
	    downloadUrl: string;
	    releaseNotes: string;
	    publishedAt: string;
	    fileSize: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasUpdate = source["hasUpdate"];
	        this.currentVer = source["currentVer"];
	        this.latestVer = source["latestVer"];
	        this.downloadUrl = source["downloadUrl"];
	        this.releaseNotes = source["releaseNotes"];
	        this.publishedAt = source["publishedAt"];
	        this.fileSize = source["fileSize"];
	    }
	}

}

