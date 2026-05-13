export namespace config {
	
	export class ServerProfile {
	    id: string;
	    name: string;
	    private_key: string;
	    public_key: string;
	    assigned_ip: string;
	    server_public_key: string;
	    server_endpoint: string;
	    status: string;
	    dns: string;
	    base_subnet: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.private_key = source["private_key"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.server_public_key = source["server_public_key"];
	        this.server_endpoint = source["server_endpoint"];
	        this.status = source["status"];
	        this.dns = source["dns"];
	        this.base_subnet = source["base_subnet"];
	    }
	}
	export class ClientVault {
	    device_id: string;
	    use_global_routing: boolean;
	    profiles: Record<string, ServerProfile>;
	    active_profile_id: string;
	    private_key?: string;
	    public_key?: string;
	    assigned_ip?: string;
	    server_public_key?: string;
	    server_endpoint?: string;
	    status?: string;
	    dns?: string;
	    base_subnet?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientVault(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device_id = source["device_id"];
	        this.use_global_routing = source["use_global_routing"];
	        this.profiles = this.convertValues(source["profiles"], ServerProfile, true);
	        this.active_profile_id = source["active_profile_id"];
	        this.private_key = source["private_key"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.server_public_key = source["server_public_key"];
	        this.server_endpoint = source["server_endpoint"];
	        this.status = source["status"];
	        this.dns = source["dns"];
	        this.base_subnet = source["base_subnet"];
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

