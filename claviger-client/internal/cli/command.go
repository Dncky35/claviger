package cli

import (
	"fmt"
)

func PrintHelp() {
	fmt.Print(`
🛡️  Claviger Zero Trust Engine

Usage:
  claviger-client generate         - Generate a new Passport token to join a network
  claviger-client approve <token>  - Apply a Visa token provided by your Administrator
  claviger-client autoconnect      - Set enable/disable auto-connect feature
  claviger-client global           - Set enable/disable global routing feature
  claviger-client list             - Show all enrolled server profiles
  
  claviger-client remove <id>      - Delete a server profile from this device 
  claviger-client connect [id] 

  claviger-client disconnect       - Gracefully shut down the active VPN connection
  claviger-client status           - Provide current status and config of the client 
  claviger-client daemon           - Start the background VPN engine (Used by systemd)
  claviger-client update		   - Check and done the update
`)
}
