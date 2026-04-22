package network

import (
	"database/sql"
	"log"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes" // This is now used properly below!
)

// PruneZombiePeers finds peers that haven't connected in X days and pauses them.
func PruneZombiePeers(db *sql.DB, daysThreshold int) (int, error) {
	log.Printf("[Cleanup] 🧟 Scanning for zombie peers (older than %d days)...", daysThreshold)

	wg, err := wgctrl.New()
	if err != nil {
		return 0, err
	}
	defer wg.Close()

	device, err := wg.Device("wg0")
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -daysThreshold)
	zombiesFound := 0

	for _, peer := range device.Peers {
		// If the peer has never handshaked, its time is zero.
		// We only target peers that WERE active but have gone cold.
		if !peer.LastHandshakeTime.IsZero() && peer.LastHandshakeTime.Before(cutoff) {
			pubKey := peer.PublicKey.String()
			log.Printf("[Cleanup] 🪓 Found zombie peer: %s (Last seen: %v)", pubKey, peer.LastHandshakeTime)

			// 1. Mark as 'paused' in the database
			_, err := db.Exec("UPDATE clients SET status = 'paused' WHERE public_key = ?", pubKey)
			if err != nil {
				log.Printf("⚠️ Error updating DB for zombie %s: %v", pubKey, err)
				continue
			}

			// 2. Hot-remove from the kernel immediately using wgtypes!
			err = wg.ConfigureDevice("wg0", wgtypes.Config{
				Peers: []wgtypes.PeerConfig{
					{
						PublicKey: peer.PublicKey,
						Remove:    true,
					},
				},
			})
			if err != nil {
				log.Printf("⚠️ Error removing zombie from kernel: %v", err)
			}

			zombiesFound++
		}
	}

	return zombiesFound, nil
}
