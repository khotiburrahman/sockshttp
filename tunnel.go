func (t *SSHTunnel) pinger(client *ssh.Client) {
	for {
		if !t.connected.Load() {
			return
		}

		// Hitung interval acak (Jitter) antara Pinger dan PingerMax
		minSec := t.cfg.Pinger
		maxSec := t.cfg.PingerMax
		
		// Validasi: jika max <= min, anggap fixed interval (backward-compat)
		if maxSec <= minSec {
			maxSec = minSec
		}

		var waitSec int
		if maxSec > minSec {
			// Random antara min dan max (inclusive)
			rangeSize := int64(maxSec - minSec)
			nBig, err := rand.Int(rand.Reader, big.NewInt(rangeSize+1))
			if err == nil {
				waitSec = minSec + int(nBig.Int64())
			} else {
				waitSec = minSec
			}
		} else {
			waitSec = minSec
		}

		// Debug log untuk memastikan jitter berjalan
		t.logger.Debug("[pinger] Menunggu %d detik sebelum ping berikutnya", waitSec)
		time.Sleep(time.Duration(waitSec) * time.Second)

		// Cek lagi setelah tidur, mungkin tunnel sudah mati
		if !t.connected.Load() {
			return
		}

		// Kirim ping
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			t.logger.Warn("Ping gagal → paksa reconnect: %v", err)
			client.Close()
			select {
			case t.disconCh <- err:
			default:
			}
			return
		}
	}
}