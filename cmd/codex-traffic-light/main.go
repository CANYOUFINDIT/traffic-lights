package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const (
	defaultBaudRate = 115200
	toolName        = "codex-traffic-light"
	logFileName     = "events.jsonl"
)

var validCommands = map[string]bool{
	"red":          true,
	"yellow":       true,
	"green":        true,
	"off":          true,
	"blink_red":    true,
	"blink_yellow": true,
	"blink_green":  true,
	"status":       true,
}

var nonzeroExitPattern = regexp.MustCompile(`(?i)(exit(?:ed)?(?:\s+status|\s+code|\s+with\s+code)?|return\s+code)[:= ]+(-?[1-9][0-9]*)`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		if installerMode() {
			return setup()
		}
		printUsage()
		return nil
	}

	switch args[0] {
	case "setup":
		return setup()
	case "install":
		return installHooks()
	case "hook":
		return runHook(os.Stdin, os.Stderr)
	case "send":
		if len(args) != 2 {
			return errors.New("usage: codex-traffic-light send <green|red|yellow|blink_red|blink_yellow|blink_green|off|status>")
		}
		return sendCommand(args[1], true)
	case "doctor":
		return doctor()
	case "ports":
		return printPorts()
	case "logs":
		return printLogs(args[1:])
	case "clear-logs":
		return clearLogs()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printUsage() {
	fmt.Println(`Usage:
  codex-traffic-light setup
  codex-traffic-light install
  codex-traffic-light hook
  codex-traffic-light send <green|red|yellow|blink_red|blink_yellow|blink_green|off|status>
  codex-traffic-light doctor
  codex-traffic-light ports
  codex-traffic-light logs [count]
  codex-traffic-light clear-logs

Environment:
  CODEX_TRAFFIC_LIGHT_PORT  Override auto-detected serial port.
  CODEX_TRAFFIC_LIGHT_BAUD  Override baud rate, defaults to 115200.`)
}

func installerMode() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(exe))
	return strings.Contains(name, "installer") || strings.Contains(name, "setup")
}

func setup() error {
	fmt.Println("Codex Traffic Light installer")
	fmt.Println()
	if err := installHooks(); err != nil {
		return err
	}
	fmt.Println()
	if err := doctor(); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Done. In Codex, open /hooks once and trust the hook definition.")
	return nil
}

func installHooks() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	exe, _ = filepath.Abs(exe)

	codexHome, err := codexHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(codexHome, 0755); err != nil {
		return fmt.Errorf("create %s: %w", codexHome, err)
	}

	installedExe, err := installExecutable(exe, codexHome)
	if err != nil {
		return err
	}

	hooksPath := filepath.Join(codexHome, "hooks.json")
	root := map[string]any{}
	if data, err := os.ReadFile(hooksPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse existing %s: %w", hooksPath, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", hooksPath, err)
	}

	hooks := ensureMap(root, "hooks")
	handler := map[string]any{
		"type":    "command",
		"command": shellCommand(installedExe, "hook"),
		"timeout": 5,
	}
	if runtime.GOOS == "windows" {
		handler["commandWindows"] = windowsCommand(installedExe, "hook")
	}

	for _, event := range []string{"UserPromptSubmit", "PermissionRequest", "PostToolUse", "Stop"} {
		groups := cleanTrafficLightGroups(asSlice(hooks[event]))
		groups = append(groups, map[string]any{
			"hooks": []any{handler},
		})
		hooks[event] = groups
	}
	root["hooks"] = hooks

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("render hooks json: %w", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(hooksPath, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", hooksPath, err)
	}

	fmt.Printf("Installed binary: %s\n", installedExe)
	fmt.Printf("Installed Codex hooks: %s\n", hooksPath)
	fmt.Println("Next: open /hooks in Codex once and trust the new hook definition.")
	return nil
}

func installExecutable(source string, codexHome string) (string, error) {
	installDir := filepath.Join(codexHome, "traffic-light")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("create %s: %w", installDir, err)
	}

	name := toolName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(installDir, name)
	targetAbs, _ := filepath.Abs(target)
	sourceAbs, _ := filepath.Abs(source)
	if sourceAbs == targetAbs {
		return targetAbs, nil
	}

	data, err := os.ReadFile(sourceAbs)
	if err != nil {
		return "", fmt.Errorf("read executable %s: %w", sourceAbs, err)
	}
	if err := os.WriteFile(targetAbs, data, 0755); err != nil {
		return "", fmt.Errorf("install executable %s: %w", targetAbs, err)
	}
	return targetAbs, nil
}

func runHook(in io.Reader, log io.Writer) error {
	data, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintf(log, "read hook input: %v\n", err)
		fmt.Println("{}")
		return nil
	}

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		fmt.Fprintf(log, "parse hook input: %v\n", err)
		fmt.Println("{}")
		return nil
	}

	command := commandForEvent(event)
	sendErr := ""
	if command != "" {
		if err := sendCommand(command, false); err != nil {
			sendErr = err.Error()
			fmt.Fprintf(log, "send %s: %v\n", command, err)
		}
	}
	if err := writeHookLog(event, command, sendErr); err != nil {
		fmt.Fprintf(log, "write hook log: %v\n", err)
	}

	fmt.Println("{}")
	return nil
}

func commandForEvent(event map[string]any) string {
	name := stringField(event, "hook_event_name")
	if name == "" {
		name = stringField(event, "hookEventName")
	}

	switch name {
	case "UserPromptSubmit":
		return "yellow"
	case "PermissionRequest":
		return "blink_red"
	case "PostToolUse":
		if postToolFailed(event) {
			return "blink_red"
		}
		return ""
	case "Stop":
		return "blink_green"
	default:
		return ""
	}
}

func postToolFailed(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			k := strings.ToLower(key)
			switch k {
			case "success", "ok":
				if b, ok := item.(bool); ok && !b {
					return true
				}
			case "exit_code", "exitcode", "return_code", "returncode":
				if n, ok := numberValue(item); ok && n != 0 {
					return true
				}
			case "exit_status", "exitstatus", "status_code", "statuscode", "code":
				if n, ok := numberValue(item); ok && n != 0 {
					return true
				}
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" && strings.TrimSpace(s) != "0" {
					return true
				}
			case "status", "state":
				if s, ok := item.(string); ok {
					switch strings.ToLower(s) {
					case "failed", "failure", "error", "errored", "cancelled", "canceled", "denied":
						return true
					}
				}
			case "error", "error_message", "err":
				if isMeaningfulError(item) {
					return true
				}
			}
			if postToolFailed(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if postToolFailed(item) {
				return true
			}
		}
	case string:
		return len(stringFailureSignals("", v)) > 0
	}
	return false
}

type hookLogEntry struct {
	Time                string   `json:"time"`
	Event               string   `json:"event,omitempty"`
	Command             string   `json:"command,omitempty"`
	TopLevelKeys        []string `json:"top_level_keys,omitempty"`
	Tool                string   `json:"tool,omitempty"`
	ToolResponseType    string   `json:"tool_response_type,omitempty"`
	ToolResponseKeys    []string `json:"tool_response_keys,omitempty"`
	StatusScalars       []string `json:"status_scalars,omitempty"`
	FailureDetected     bool     `json:"failure_detected,omitempty"`
	FailureSignals      []string `json:"failure_signals,omitempty"`
	CancelSignals       []string `json:"cancel_signals,omitempty"`
	InterruptSignals    []string `json:"interrupt_signals,omitempty"`
	PermissionSignals   []string `json:"permission_signals,omitempty"`
	SerialSendError     string   `json:"serial_send_error,omitempty"`
	TrafficLightVersion string   `json:"traffic_light_version"`
}

func writeHookLog(event map[string]any, command string, sendErr string) error {
	path, err := hookLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	entry := summarizeHookEvent(event, command, sendErr)
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("render hook log: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open hook log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write hook log: %w", err)
	}
	return nil
}

func summarizeHookEvent(event map[string]any, command string, sendErr string) hookLogEntry {
	entry := hookLogEntry{
		Time:                time.Now().Format(time.RFC3339),
		Event:               hookEventName(event),
		Command:             command,
		TopLevelKeys:        mapKeys(event),
		Tool:                firstStringField(event, "tool_name", "toolName", "tool", "matcher"),
		ToolResponseType:    valueType(event["tool_response"]),
		ToolResponseKeys:    nestedKeys(event["tool_response"], "tool_response", 4),
		StatusScalars:       statusScalars("", event, 0),
		SerialSendError:     sendErr,
		TrafficLightVersion: "1",
	}
	collectEventSignals("", event, &entry, 0)
	entry.FailureSignals = uniqueSorted(entry.FailureSignals)
	entry.CancelSignals = uniqueSorted(entry.CancelSignals)
	entry.InterruptSignals = uniqueSorted(entry.InterruptSignals)
	entry.PermissionSignals = uniqueSorted(entry.PermissionSignals)
	entry.FailureDetected = len(entry.FailureSignals) > 0 || len(entry.CancelSignals) > 0 || len(entry.InterruptSignals) > 0
	return entry
}

func collectEventSignals(path string, value any, entry *hookLogEntry, depth int) {
	if depth > 6 {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			nextPath := joinPath(path, key)
			lowerKey := strings.ToLower(key)

			switch lowerKey {
			case "success", "ok":
				if b, ok := item.(bool); ok && !b {
					entry.FailureSignals = append(entry.FailureSignals, nextPath+" false")
				}
			case "exit_code", "exitcode", "return_code", "returncode":
				if n, ok := numberValue(item); ok && n != 0 {
					entry.FailureSignals = append(entry.FailureSignals, nextPath+" nonzero")
				}
			case "exit_status", "exitstatus", "status_code", "statuscode", "code":
				if n, ok := numberValue(item); ok && n != 0 {
					entry.FailureSignals = append(entry.FailureSignals, nextPath+" nonzero")
				}
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" && strings.TrimSpace(s) != "0" {
					entry.FailureSignals = append(entry.FailureSignals, nextPath+" nonzero")
				}
			case "status", "state":
				if s, ok := item.(string); ok {
					recordStatusSignal(nextPath, s, entry)
				}
			case "error", "error_message", "err":
				if isMeaningfulError(item) {
					entry.FailureSignals = append(entry.FailureSignals, nextPath+" present")
				}
			case "cancelled", "canceled", "cancelled_by_user", "canceled_by_user":
				if truthy(item) {
					entry.CancelSignals = append(entry.CancelSignals, nextPath+" true")
				}
			case "interrupted", "user_interrupted", "interrupt":
				if truthy(item) {
					entry.InterruptSignals = append(entry.InterruptSignals, nextPath+" true")
				}
			case "permission", "permission_request", "approval", "approval_request":
				entry.PermissionSignals = append(entry.PermissionSignals, nextPath+" present")
			case "reason", "message":
				if s, ok := item.(string); ok {
					recordReasonSignal(nextPath, s, entry)
				}
			}

			collectEventSignals(nextPath, item, entry, depth+1)
		}
	case []any:
		for i, item := range v {
			collectEventSignals(path+"["+strconv.Itoa(i)+"]", item, entry, depth+1)
		}
	case string:
		entry.FailureSignals = append(entry.FailureSignals, stringFailureSignals(path, v)...)
	}
}

func stringFailureSignals(path string, value string) []string {
	lower := strings.ToLower(value)
	var signals []string
	if nonzeroExitPattern.MatchString(value) {
		signals = append(signals, path+" contains nonzero exit")
	}
	for _, needle := range []string{
		"no such file or directory",
		"command not found",
		"permission denied",
		"operation not permitted",
		"traceback",
		"uncaught exception",
		"error:",
		"failed:",
		"fatal:",
	} {
		if strings.Contains(lower, needle) {
			signals = append(signals, path+" contains "+needle)
		}
	}
	return signals
}

func recordStatusSignal(path string, value string, entry *hookLogEntry) {
	status := strings.ToLower(strings.TrimSpace(value))
	switch status {
	case "failed", "failure", "error", "errored", "denied", "rejected":
		entry.FailureSignals = append(entry.FailureSignals, path+"="+status)
	case "cancelled", "canceled":
		entry.CancelSignals = append(entry.CancelSignals, path+"="+status)
	case "interrupted", "interrupt":
		entry.InterruptSignals = append(entry.InterruptSignals, path+"="+status)
	}
}

func recordReasonSignal(path string, value string, entry *hookLogEntry) {
	reason := strings.ToLower(value)
	switch {
	case strings.Contains(reason, "cancel"):
		entry.CancelSignals = append(entry.CancelSignals, path+" contains cancel")
	case strings.Contains(reason, "interrupt"):
		entry.InterruptSignals = append(entry.InterruptSignals, path+" contains interrupt")
	case strings.Contains(reason, "fail") || strings.Contains(reason, "error") || strings.Contains(reason, "denied"):
		entry.FailureSignals = append(entry.FailureSignals, path+" contains failure")
	}
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1", "cancelled", "canceled", "interrupted":
			return true
		}
	case float64:
		return v != 0
	case int:
		return v != 0
	}
	return false
}

func hookEventName(event map[string]any) string {
	name := stringField(event, "hook_event_name")
	if name == "" {
		name = stringField(event, "hookEventName")
	}
	return name
}

func firstStringField(event map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(event, key); value != "" {
			return value
		}
	}
	return ""
}

func valueType(value any) string {
	switch value.(type) {
	case nil:
		return ""
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "bool"
	case float64, float32, int, int64, json.Number:
		return "number"
	default:
		return "other"
	}
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nestedKeys(value any, path string, depth int) []string {
	if depth <= 0 {
		return nil
	}
	var keys []string
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			nextPath := joinPath(path, key)
			keys = append(keys, nextPath)
			keys = append(keys, nestedKeys(item, nextPath, depth-1)...)
		}
	case []any:
		if len(v) > 0 {
			keys = append(keys, path+"[]")
			keys = append(keys, nestedKeys(v[0], path+"[]", depth-1)...)
		}
	}
	return uniqueSorted(keys)
}

func statusScalars(path string, value any, depth int) []string {
	if depth > 5 {
		return nil
	}
	var scalars []string
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			nextPath := joinPath(path, key)
			lowerKey := strings.ToLower(key)
			if isStatusLikeKey(lowerKey) {
				if scalar, ok := safeScalar(item); ok {
					scalars = append(scalars, nextPath+"="+scalar)
				}
			}
			scalars = append(scalars, statusScalars(nextPath, item, depth+1)...)
		}
	case []any:
		for i, item := range v {
			if i >= 3 {
				break
			}
			scalars = append(scalars, statusScalars(path+"["+strconv.Itoa(i)+"]", item, depth+1)...)
		}
	}
	return uniqueSorted(scalars)
}

func isStatusLikeKey(key string) bool {
	for _, needle := range []string{
		"success", "ok", "exit", "status", "state", "code", "error", "err", "cancel", "interrupt", "denied", "reason",
	} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func safeScalar(value any) (string, bool) {
	switch v := value.(type) {
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return "", false
		}
		lower := strings.ToLower(trimmed)
		for _, allowed := range []string{
			"success", "ok", "failed", "failure", "error", "errored", "cancelled", "canceled", "interrupted", "denied", "rejected", "0", "1", "true", "false",
		} {
			if lower == allowed {
				return lower, true
			}
		}
		if len(trimmed) <= 16 && strings.IndexFunc(trimmed, func(r rune) bool {
			return !(r == '-' || r == '_' || r == '.' || r == ':' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
		}) == -1 {
			return trimmed, true
		}
		return "present", true
	default:
		if isMeaningfulError(value) {
			return "present", true
		}
		return "", false
	}
}

func joinPath(parent string, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	sort.Strings(unique)
	return unique
}

func hookLogPath() (string, error) {
	codexHome, err := codexHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(codexHome, "traffic-light", logFileName), nil
}

func printLogs(args []string) error {
	count := 20
	if len(args) > 1 {
		return errors.New("usage: codex-traffic-light logs [count]")
	}
	if len(args) == 1 {
		parsed, err := strconv.Atoi(args[0])
		if err != nil || parsed <= 0 {
			return errors.New("log count must be a positive integer")
		}
		count = parsed
	}

	path, err := hookLogPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("No hook log yet: %s\n", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read hook log: %w", err)
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) == 1 && len(lines[0]) == 0 {
		fmt.Printf("Hook log is empty: %s\n", path)
		return nil
	}
	start := len(lines) - count
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Println(string(line))
	}
	return nil
}

func clearLogs() error {
	path, err := hookLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		return fmt.Errorf("clear hook log: %w", err)
	}
	fmt.Printf("Cleared hook log: %s\n", path)
	return nil
}

func isMeaningfulError(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case bool:
		return v
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

func sendCommand(command string, verbose bool) error {
	command = strings.TrimSpace(strings.ToLower(command))
	if !validCommands[command] {
		return fmt.Errorf("invalid light command: %s", command)
	}

	portName, err := configuredOrDetectedPort()
	if err != nil {
		return err
	}

	baud := baudRate()
	port, err := serial.Open(portName, &serial.Mode{BaudRate: baud})
	if err != nil {
		return fmt.Errorf("open %s: %w", portName, err)
	}
	defer port.Close()

	_ = port.SetReadTimeout(700 * time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	_, _ = port.Read(make([]byte, 256))

	if _, err := port.Write([]byte(command + "\n")); err != nil {
		return fmt.Errorf("write %s: %w", portName, err)
	}
	_ = port.Drain()

	if verbose {
		fmt.Printf("sent %q to %s\n", command, portName)
		reply := make([]byte, 256)
		n, _ := port.Read(reply)
		if n > 0 {
			fmt.Print(strings.TrimSpace(string(reply[:n])))
			fmt.Println()
		}
	}
	return nil
}

func configuredOrDetectedPort() (string, error) {
	if port := strings.TrimSpace(os.Getenv("CODEX_TRAFFIC_LIGHT_PORT")); port != "" {
		return port, nil
	}

	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", fmt.Errorf("list serial ports: %w", err)
	}

	for _, port := range ports {
		if isEsp32C3(port) {
			return port.Name, nil
		}
	}
	for _, port := range ports {
		if looksLikeUsbSerial(port) {
			return port.Name, nil
		}
	}
	return "", errors.New("no ESP32-C3 serial port found; set CODEX_TRAFFIC_LIGHT_PORT to override")
}

func isEsp32C3(port *enumerator.PortDetails) bool {
	if port == nil || !port.IsUSB {
		return false
	}
	vid := strings.ToUpper(port.VID)
	pid := strings.ToUpper(port.PID)
	if vid == "303A" && pid == "1001" {
		return true
	}
	text := strings.ToLower(port.Name + " " + port.Product + " " + port.SerialNumber)
	return strings.Contains(text, "esp") && strings.Contains(text, "serial")
}

func looksLikeUsbSerial(port *enumerator.PortDetails) bool {
	if port == nil {
		return false
	}
	text := strings.ToLower(port.Name + " " + port.Product + " " + port.SerialNumber)
	if strings.Contains(text, "bluetooth") || strings.Contains(text, "debug-console") {
		return false
	}
	for _, needle := range []string{"usbmodem", "usbserial", "wchusbserial", "ch340", "cp210", "jtag/serial", "usb jtag"} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func baudRate() int {
	raw := strings.TrimSpace(os.Getenv("CODEX_TRAFFIC_LIGHT_BAUD"))
	if raw == "" {
		return defaultBaudRate
	}
	baud, err := strconv.Atoi(raw)
	if err != nil || baud <= 0 {
		return defaultBaudRate
	}
	return baud
}

func doctor() error {
	fmt.Printf("%s doctor\n", toolName)
	fmt.Printf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if port := strings.TrimSpace(os.Getenv("CODEX_TRAFFIC_LIGHT_PORT")); port != "" {
		fmt.Printf("Port override: %s\n", port)
	}

	codexHome, err := codexHome()
	if err != nil {
		return err
	}
	hooksPath := filepath.Join(codexHome, "hooks.json")
	fmt.Printf("Codex hooks: %s\n", hooksPath)
	if data, err := os.ReadFile(hooksPath); err == nil && bytes.Contains(data, []byte(toolName)) {
		fmt.Println("Hook config: installed")
	} else {
		fmt.Println("Hook config: not installed")
	}

	fmt.Println("Serial ports:")
	if err := printPorts(); err != nil {
		return err
	}

	port, err := configuredOrDetectedPort()
	if err != nil {
		fmt.Printf("Traffic light: not found (%v)\n", err)
		return nil
	}
	fmt.Printf("Traffic light: %s\n", port)
	return nil
}

func printPorts() error {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return fmt.Errorf("list serial ports: %w", err)
	}
	if len(ports) == 0 {
		fmt.Println("  none")
		return nil
	}
	for _, port := range ports {
		parts := []string{port.Name}
		if port.IsUSB {
			if port.VID != "" || port.PID != "" {
				parts = append(parts, "VID:PID="+strings.ToUpper(port.VID)+":"+strings.ToUpper(port.PID))
			}
			if port.Product != "" {
				parts = append(parts, "product="+port.Product)
			}
			if port.SerialNumber != "" {
				parts = append(parts, "serial="+port.SerialNumber)
			}
		}
		if isEsp32C3(port) {
			parts = append(parts, "candidate=esp32-c3")
		} else if looksLikeUsbSerial(port) {
			parts = append(parts, "candidate=usb-serial")
		}
		fmt.Println("  " + strings.Join(parts, " "))
	}
	return nil
}

func codexHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func ensureMap(root map[string]any, key string) map[string]any {
	if existing, ok := root[key].(map[string]any); ok {
		return existing
	}
	next := map[string]any{}
	root[key] = next
	return next
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func cleanTrafficLightGroups(groups []any) []any {
	cleaned := make([]any, 0, len(groups))
	for _, group := range groups {
		groupMap, ok := group.(map[string]any)
		if !ok {
			cleaned = append(cleaned, group)
			continue
		}
		hooks := asSlice(groupMap["hooks"])
		nextHooks := make([]any, 0, len(hooks))
		for _, hook := range hooks {
			if !isTrafficLightHook(hook) {
				nextHooks = append(nextHooks, hook)
			}
		}
		if len(nextHooks) == 0 && len(hooks) > 0 {
			continue
		}
		groupMap["hooks"] = nextHooks
		cleaned = append(cleaned, groupMap)
	}
	return cleaned
}

func isTrafficLightHook(value any) bool {
	hook, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"command", "commandWindows"} {
		if command, ok := hook[key].(string); ok {
			compact := strings.ReplaceAll(command, ".exe", "")
			if strings.Contains(compact, toolName) && strings.Contains(compact, "hook") {
				return true
			}
		}
	}
	return false
}

func shellCommand(exe string, arg string) string {
	if runtime.GOOS == "windows" {
		return windowsCommand(exe, arg)
	}
	return quoteUnix(exe) + " " + quoteUnix(arg)
}

func quoteUnix(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func windowsCommand(exe string, arg string) string {
	return quoteWindows(exe) + " " + quoteWindows(arg)
}

func quoteWindows(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func stringField(value map[string]any, key string) string {
	raw, _ := value[key].(string)
	return raw
}
