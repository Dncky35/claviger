package cli

import (
	"fmt"
	"os"
	"os/exec"
)

func HandleUpdate() {
	fmt.Println("🚀 Initializing secure Claviger update...")
	cmd := exec.Command("bash", "-c", "curl -sSL https://cloudrocean.com/installers/claviger-client.sh | sudo bash")

	// Bind the output so the user sees the bash script's progress in their terminal
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("❌ Update failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
