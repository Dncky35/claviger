package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"claviger-server/storage"
)

func RunReset() {
	fmt.Println("⚠️  Resetting Claviger Edge Node...")

	// The interactive safety prompt
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Are you sure you want to reset this node? This will wipe the local configuration. [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	// If they type anything other than 'y' or 'yes', abort.
	if input != "y" && input != "yes" {
		fmt.Println("❌ Reset cancelled. Local configuration was not changed.")
		return
	}

	db := storage.InitDB()
	defer db.Close()

	storage.ClearConfig(db)

	fmt.Println("✅ Local configuration wiped.")
	fmt.Println("✅ You can now run 'claviger setup --key <new-key>' to re-register.")
}
