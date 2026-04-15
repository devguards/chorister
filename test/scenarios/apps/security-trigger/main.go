// security-trigger: intentionally performs detectable actions for Tetragon testing.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
)

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

// execShell forks a shell (triggers Tetragon process exec event).
func execShell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	cmd := exec.Command("/bin/sh", "-c", "echo hi")
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("exec failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("exec-shell: %s", string(out))
	fmt.Fprintf(w, "exec output: %s\n", string(out))
}

// writeSensitive writes to /etc/trigger-test (triggers Tetragon file write event).
func writeSensitive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := os.WriteFile("/etc/trigger-test", []byte("triggered\n"), 0600); err != nil {
		http.Error(w, fmt.Sprintf("write failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Println("write-sensitive: wrote to /etc/trigger-test")
	fmt.Fprintln(w, "wrote to /etc/trigger-test")
}

// tcpScan opens connections to random IPs (triggers egress anomaly).
func tcpScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	targets := []string{
		"10.0.0.1:80", "10.0.0.2:80", "10.0.0.3:80",
		"10.0.0.4:80", "10.0.0.5:80",
	}
	attempted := 0
	for _, t := range targets {
		conn, err := net.DialTimeout("tcp", t, 200*1000*1000) // 200ms
		if err == nil {
			conn.Close()
		}
		attempted++
	}
	log.Printf("tcp-scan: attempted %d connections", attempted)
	fmt.Fprintf(w, "scanned %d targets\n", attempted)
}

func main() {
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/exec-shell", execShell)
	http.HandleFunc("/write-sensitive", writeSensitive)
	http.HandleFunc("/tcp-scan", tcpScan)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("security-trigger listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
