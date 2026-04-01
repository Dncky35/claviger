export namespace config {
	
	export class ClientVault {
	    server_url: string;
	    client_id: string;
	    private_key: string;
	    public_key: string;
	    assigned_ip: string;
	    server_public_key: string;
	    server_endpoint: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientVault(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.client_id = source["client_id"];
	        this.private_key = source["private_key"];
	        this.public_key = source["public_key"];
	        this.assigned_ip = source["assigned_ip"];
	        this.server_public_key = source["server_public_key"];
	        this.server_endpoint = source["server_endpoint"];
	        this.status = source["status"];
	    }
	}

}

