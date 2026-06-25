package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

const version = "1.0.2"

var (
	bold    = color.New(color.Bold).SprintFunc()
	green   = color.New(color.FgGreen).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	yellow  = color.New(color.FgYellow).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
	dim     = color.New(color.Faint).SprintFunc()
)

type Config struct {
	AuthURL  string `yaml:"auth_url"`
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
	Profile  string `yaml:"profile"`
	Region   string `yaml:"region"`
}

func loadConfig() *Config {
	cfg := &Config{}

	// Check env vars first
	if url := os.Getenv("SWS_API_URL"); url != "" {
		cfg.AuthURL = url
	}
	if token := os.Getenv("SWS_TOKEN"); token != "" {
		cfg.Token = token
	}
	if region := os.Getenv("SWS_REGION"); region != "" {
		cfg.Region = region
	}

	// Load config file
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".config", "sws", "credentials.yaml"),
		filepath.Join(home, ".sws", "config.yaml"),
		"sws.yaml",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var root map[string]map[string]interface{}
		yaml.Unmarshal(data, &root)
		profile := os.Getenv("SWS_PROFILE")
		if profile == "" {
			profile = "default"
		}
		if cloud, ok := root["clouds"]; ok {
			if prof, ok := cloud[profile]; ok {
				if m, ok := prof.(map[string]interface{}); ok {
					// File values fill in only what the environment didn't set —
					// env vars take precedence (aws-cli behaviour).
					if auth, ok := m["auth"].(map[string]interface{}); ok {
						if v, ok := auth["auth_url"].(string); ok && cfg.AuthURL == "" {
							cfg.AuthURL = v
						}
						if v, ok := auth["token"].(string); ok && cfg.Token == "" {
							cfg.Token = v
						}
						if v, ok := auth["username"].(string); ok && cfg.Email == "" {
							cfg.Email = v
						}
						if v, ok := auth["password"].(string); ok && cfg.Password == "" {
							cfg.Password = v
						}
					}
					if v, ok := m["region_name"].(string); ok && cfg.Region == "" {
						cfg.Region = v
					}
				}
			}
		}
		break
	}

	return cfg
}

func (c *Config) ensureToken() error {
	if c.Token != "" {
		return nil
	}
	if c.AuthURL == "" || c.Email == "" || c.Password == "" {
		return fmt.Errorf("not authenticated. Run: sws login")
	}

	baseURL := strings.TrimSuffix(c.AuthURL, "/api/identity")
	body, _ := json.Marshal(map[string]string{"email": c.Email, "password": c.Password})
	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := httpClient.Post(baseURL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if token, ok := result["token"].(string); ok {
		c.Token = token
		return nil
	}
	if errMsg, ok := result["error"].(string); ok {
		return fmt.Errorf("login failed: %s", errMsg)
	}
	return fmt.Errorf("login failed")
}

func (c *Config) apiURL() string {
	base := strings.TrimSuffix(c.AuthURL, "/api/identity")
	if base == "" {
		base = "https://savannaa.com"
	}
	return base
}

func (c *Config) request(method, path string, body interface{}) (map[string]interface{}, []interface{}, error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.apiURL()+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	// Route the request to the caller's region (e.g. ng-abuja-1 | ng-lagos-1).
	// Without this the API falls back to the default region and a user whose
	// resources live in the other region sees an empty list.
	if c.Region != "" {
		req.Header.Set("x-region", c.Region)
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("error %d: %s", resp.StatusCode, string(data))
	}

	// Try as array first
	var arr []interface{}
	if json.Unmarshal(data, &arr) == nil {
		return nil, arr, nil
	}

	// Try as object
	var obj map[string]interface{}
	json.Unmarshal(data, &obj)
	return obj, nil, nil
}

func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	// Header
	h := make([]string, len(headers))
	for i, hdr := range headers {
		h[i] = bold(strings.ToUpper(hdr))
	}
	fmt.Fprintln(w, strings.Join(h, "\t"))
	// Rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func str(v interface{}) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%v", v)
}

func truncID(id string) string {
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cfg := loadConfig()

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("sws CLI %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	case "login":
		cmdLogin(cfg, args)
	case "configure":
		cmdConfigure(cfg, args)
	case "compute":
		cmdCompute(cfg, args)
	case "network":
		cmdNetwork(cfg, args)
	case "volume":
		cmdVolume(cfg, args)
	case "database":
		cmdDatabase(cfg, args)
	case "container":
		cmdContainer(cfg, args)
	case "cluster":
		cmdCluster(cfg, args)
	case "secret":
		cmdSecret(cfg, args)
	case "lb":
		cmdLB(cfg, args)
	case "ip":
		cmdIP(cfg, args)
	case "firewall":
		cmdFirewall(cfg, args)
	case "image":
		cmdImage(cfg, args)
	default:
		fmt.Fprintf(os.Stderr, "%s Unknown command: %s\n", red("Error:"), cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`%s - Cloud Platform CLI v%s

%s
  sws <command> <subcommand> [options]

%s
  login                     Authenticate with the platform
  configure                 Set up credentials + region (~/.sws/config.yaml)
  compute                   Manage virtual machines
  network                   Manage networks
  volume                    Manage block storage volumes
  ip                        Manage public IPs
  firewall                  Manage firewall rules
  image                     Manage OS images
  database                  Manage database instances
  container                 Manage serverless containers
  cluster                   Manage Kubernetes clusters
  secret                    Manage secrets and keys
  lb                        Manage load balancers
  version                   Show CLI version
  help                      Show this help

%s
  sws compute list
  sws compute create --name web-1 --image "Ubuntu 24.04 LTS" --plan m1.small
  sws volume create --name data --size 50
  sws container run --name api --image python:3.12
  sws cluster create prod-k8s --template kubernetes-default --workers 3

%s
  SWS_API_URL    Base URL (e.g., https://savannaa.com)
  SWS_TOKEN      Auth token (skip login)
  SWS_PROFILE    Config profile name (default: "default")
  SWS_REGION     Region to target: ng-abuja-1 | ng-lagos-1 (default: ng-lagos-1)
`, bold("sws"), version,
		bold("USAGE:"), bold("COMMANDS:"), bold("EXAMPLES:"), bold("ENVIRONMENT:"))
}

func cmdLogin(cfg *Config, args []string) {
	if len(args) >= 2 {
		cfg.Email = args[0]
		cfg.Password = args[1]
	} else {
		fmt.Print("Email: ")
		fmt.Scanln(&cfg.Email)
		fmt.Print("Password: ")
		fmt.Scanln(&cfg.Password)
	}
	if cfg.AuthURL == "" {
		fmt.Print("API URL: ")
		fmt.Scanln(&cfg.AuthURL)
	}

	if err := cfg.ensureToken(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Login failed:"), err)
		os.Exit(1)
	}
	fmt.Printf("%s Authenticated as %s\n", green("✓"), cyan(cfg.Email))
	fmt.Printf("  Token: %s...%s\n", cfg.Token[:20], cfg.Token[len(cfg.Token)-10:])

	// Save token
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "sws")
	os.MkdirAll(dir, 0700)
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte(cfg.Token), 0600)
	fmt.Printf("  Token saved to %s\n", dim(tokenFile))
}

// cmdConfigure is an aws-configure-style interactive setup. It prompts for the
// API URL, an API token (ctk_…), and the default region, then writes them to
// ~/.sws/config.yaml under the active profile (SWS_PROFILE, default "default"),
// preserving any other profiles already in the file. Press Enter at a prompt
// to keep the current value shown in [brackets].
func cmdConfigure(cfg *Config, args []string) {
	reader := bufio.NewReader(os.Stdin)
	ask := func(label, current string) string {
		shown := current
		if shown == "" {
			shown = "None"
		}
		fmt.Printf("%s [%s]: ", label, shown)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return current
		}
		return line
	}

	profile := os.Getenv("SWS_PROFILE")
	if profile == "" {
		profile = "default"
	}

	apiURL := cfg.AuthURL
	if apiURL == "" {
		apiURL = "https://savannaa.com"
	}
	region := cfg.Region
	if region == "" {
		region = "ng-lagos-1"
	}

	apiURL = ask("API URL", apiURL)

	// Show only the tail of an existing token; blank input keeps it.
	tokenShown := cfg.Token
	if len(tokenShown) > 8 {
		tokenShown = "****" + tokenShown[len(tokenShown)-4:]
	}
	tokenInput := ask("API token (ctk_...)", tokenShown)
	token := cfg.Token
	if tokenInput != tokenShown {
		token = tokenInput
	}

	region = ask("Default region (ng-abuja-1 | ng-lagos-1)", region)

	// Preserve other profiles already in the file.
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".sws")
	path := filepath.Join(dir, "config.yaml")
	root := map[string]map[string]interface{}{}
	if data, err := os.ReadFile(path); err == nil {
		yaml.Unmarshal(data, &root)
	}
	if root == nil {
		root = map[string]map[string]interface{}{}
	}
	clouds := root["clouds"]
	if clouds == nil {
		clouds = map[string]interface{}{}
	}
	clouds[profile] = map[string]interface{}{
		"auth": map[string]interface{}{
			"auth_url": apiURL,
			"token":    token,
		},
		"region_name": region,
	}
	root["clouds"] = clouds

	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	out, _ := yaml.Marshal(root)
	if err := os.WriteFile(path, out, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	fmt.Printf("%s Saved profile %s to %s\n", green("✓"), cyan(profile), dim(path))
}

// ── Compute ──────────────────────────────────────────

func cmdCompute(cfg *Config, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: sws compute <list|create|start|stop|delete> [options]")
		return
	}
	switch args[0] {
	case "list", "ls":
		_, arr, err := cfg.request("GET", "/api/compute/servers", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1)
		}
		if len(arr) == 0 {
			fmt.Println(dim("No instances found."))
			return
		}
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["status"]), str(m["power_state"])})
		}
		printTable([]string{"ID", "Name", "Status", "Power"}, rows)

	case "create":
		payload := parseFlags(args[1:], []string{"name", "image", "plan", "network", "key-name"})
		body := map[string]interface{}{
			"name": payload["name"], "image_id": payload["image"],
			"flavor_id": payload["plan"], "networks": []map[string]string{{"uuid": payload["network"]}},
		}
		if kn := payload["key-name"]; kn != "" {
			body["key_name"] = kn
		}
		obj, _, err := cfg.request("POST", "/api/compute/servers", body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1)
		}
		fmt.Printf("%s Instance %s created (%s)\n", green("✓"), bold(str(obj["name"])), truncID(str(obj["id"])))

	case "start":
		if len(args) < 2 { fmt.Println("Usage: sws compute start <id>"); return }
		_, _, err := cfg.request("POST", "/api/compute/servers/"+args[1]+"/start", nil)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Instance started\n", green("✓"))

	case "stop":
		if len(args) < 2 { fmt.Println("Usage: sws compute stop <id>"); return }
		_, _, err := cfg.request("POST", "/api/compute/servers/"+args[1]+"/stop", nil)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Instance stopped\n", green("✓"))

	case "delete", "rm":
		if len(args) < 2 { fmt.Println("Usage: sws compute delete <id>"); return }
		_, _, err := cfg.request("DELETE", "/api/compute/servers/"+args[1], nil)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Instance deleted\n", green("✓"))

	default:
		fmt.Printf("Unknown subcommand: compute %s\n", args[0])
	}
}

// ── Network ──────────────────────────────────────────

func cmdNetwork(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws network <list|create|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/network/networks", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["status"])})
		}
		printTable([]string{"ID", "Name", "Status"}, rows)
	default:
		fmt.Printf("Unknown: network %s\n", args[0])
	}
}

// ── Volume ───────────────────────────────────────────

func cmdVolume(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws volume <list|create|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/storage/volumes", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["status"]), str(m["size"]) + " GB"})
		}
		printTable([]string{"ID", "Name", "Status", "Size"}, rows)
	case "create":
		p := parseFlags(args[1:], []string{"name", "size"})
		body := map[string]interface{}{"name": p["name"], "size": toInt(p["size"])}
		obj, _, err := cfg.request("POST", "/api/storage/volumes", body)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Volume %s created\n", green("✓"), bold(str(obj["name"])))
	case "delete", "rm":
		if len(args) < 2 { return }
		cfg.request("DELETE", "/api/storage/volumes/"+args[1], nil)
		fmt.Printf("%s Volume deleted\n", green("✓"))
	}
}

// ── Database ─────────────────────────────────────────

func cmdDatabase(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws database <list|create|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/database/instances", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			ds := ""
			if d, ok := m["datastore"].(map[string]interface{}); ok {
				ds = str(d["type"]) + " " + str(d["version"])
			}
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), ds, str(m["status"])})
		}
		printTable([]string{"ID", "Name", "Engine", "Status"}, rows)
	}
}

// ── Container (Serverless) ───────────────────────────

func cmdContainer(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws container <list|run|stop|delete|logs>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/serverless/containers", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["uuid"])), str(m["name"]), str(m["image"]), str(m["status"])})
		}
		printTable([]string{"ID", "Name", "Image", "Status"}, rows)
	case "run":
		p := parseFlags(args[1:], []string{"name", "image", "cpu", "memory", "command"})
		body := map[string]interface{}{"name": p["name"], "image": p["image"], "interactive": true}
		if p["cpu"] != "" { body["cpu"] = toFloat(p["cpu"]) }
		if p["memory"] != "" { body["memory"] = toInt(p["memory"]) }
		if p["command"] != "" { body["command"] = p["command"] }
		_, _, err := cfg.request("POST", "/api/serverless/containers", body)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Container %s deployed\n", green("✓"), bold(p["name"]))
	case "logs":
		if len(args) < 2 { return }
		obj, _, _ := cfg.request("GET", "/api/serverless/containers/"+args[1]+"/logs", nil)
		fmt.Println(str(obj["logs"]))
	case "delete", "rm":
		if len(args) < 2 { return }
		cfg.request("DELETE", "/api/serverless/containers/"+args[1], nil)
		fmt.Printf("%s Container deleted\n", green("✓"))
	}
}

// ── Cluster (Kubernetes) ─────────────────────────────

func cmdCluster(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws cluster <list|create|delete|kubeconfig>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/orchestration/clusters", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["uuid"])), str(m["name"]), str(m["status"]), str(m["node_count"]) + " nodes"})
		}
		printTable([]string{"ID", "Name", "Status", "Nodes"}, rows)
	case "create":
		p := parseFlags(args[1:], []string{"name", "template", "workers", "masters", "keypair"})
		body := map[string]interface{}{
			"name": p["name"], "cluster_template_id": p["template"],
			"node_count": toInt(p["workers"]), "master_count": toInt(p["masters"]),
		}
		if p["keypair"] != "" { body["keypair"] = p["keypair"] }
		_, _, err := cfg.request("POST", "/api/orchestration/clusters", body)
		if err != nil { fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err); os.Exit(1) }
		fmt.Printf("%s Cluster %s creation started\n", green("✓"), bold(p["name"]))
	case "kubeconfig":
		if len(args) < 2 { return }
		obj, _, _ := cfg.request("GET", "/api/orchestration/clusters/"+args[1]+"/kubeconfig", nil)
		fmt.Print(str(obj["kubeconfig"]))
	case "delete", "rm":
		if len(args) < 2 { return }
		cfg.request("DELETE", "/api/orchestration/clusters/"+args[1], nil)
		fmt.Printf("%s Cluster deleted\n", green("✓"))
	}
}

// ── Secrets ──────────────────────────────────────────

func cmdSecret(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws secret <list|store|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/orchestration/secrets", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["secret_ref"])), str(m["name"]), str(m["status"])})
		}
		printTable([]string{"Ref", "Name", "Status"}, rows)
	}
}

// ── Load Balancer ────────────────────────────────────

func cmdLB(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws lb <list|create|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/orchestration/load-balancers", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["status"]), str(m["ip_address"])})
		}
		printTable([]string{"ID", "Name", "Status", "VIP"}, rows)
	}
}

// ── Public IP ────────────────────────────────────────

func cmdIP(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws ip <list|allocate>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/network/floating-ips", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["public_ip"]), str(m["private_ip"]), str(m["status"])})
		}
		printTable([]string{"ID", "Public IP", "Mapped To", "Status"}, rows)
	}
}

// ── Firewall ─────────────────────────────────────────

func cmdFirewall(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws firewall <list|create|delete>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/network/security-groups", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["description"])})
		}
		printTable([]string{"ID", "Name", "Description"}, rows)
	}
}

// ── Image ────────────────────────────────────────────

func cmdImage(cfg *Config, args []string) {
	if len(args) == 0 { fmt.Println("Usage: sws image <list>"); return }
	switch args[0] {
	case "list", "ls":
		_, arr, _ := cfg.request("GET", "/api/images", nil)
		rows := [][]string{}
		for _, item := range arr {
			m := item.(map[string]interface{})
			rows = append(rows, []string{truncID(str(m["id"])), str(m["name"]), str(m["status"])})
		}
		printTable([]string{"ID", "Name", "Status"}, rows)
	}
}

// ── Helpers ──────────────────────────────────────────

func parseFlags(args []string, keys []string) map[string]string {
	result := map[string]string{}
	for i := 0; i < len(args); i++ {
		for _, key := range keys {
			if args[i] == "--"+key && i+1 < len(args) {
				result[key] = args[i+1]
				i++
				break
			}
		}
		// Positional: first non-flag arg is "name"
		if !strings.HasPrefix(args[i], "--") && result["name"] == "" {
			result["name"] = args[i]
		}
	}
	return result
}

func toInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n == 0 { n = 1 }
	return n
}

func toFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	if f == 0 { f = 0.5 }
	return f
}
