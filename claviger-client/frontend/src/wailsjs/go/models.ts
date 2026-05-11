export namespace config {
	
	export class ClientVault {
	    private_key: string;
	    public_key: string;
	    assigned_ip: string;
	    server_public_key: string;
	    device_id: string;
	    server_endpoint: string;
	    status: string;
	    use_global_routing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClientVault(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.private_key = source["private_key"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.server_public_key = source["server_public_key"];
	        this.device_id = source["device_id"];
	        this.server_endpoint = source["server_endpoint"];
	        this.status = source["status"];
	        this.use_global_routing = source["use_global_routing"];
	    }
	}

}

