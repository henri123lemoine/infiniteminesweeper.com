package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Instance struct {
	Port    int
	PID     int
	Workdir string
	Branch  string
}

func findInstances() []Instance {
	// Use lsof to find processes listening on ports 8080-8099
	cmd := exec.Command("lsof", "-iTCP:8080-8099", "-sTCP:LISTEN", "-n", "-P")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var instances []Instance
	seen := make(map[int]bool)
	lines := strings.Split(string(output), "\n")

	portRegex := regexp.MustCompile(`:(\d+)$`)

	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		// Only look for our backend process
		if fields[0] != "backend" {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil || seen[pid] {
			continue
		}
		seen[pid] = true

		// Extract port - it's in the second-to-last field (e.g., "*:8080")
		// Last field is "(LISTEN)"
		addrField := fields[len(fields)-2]
		match := portRegex.FindStringSubmatch(addrField)
		if match == nil {
			continue
		}
		port, _ := strconv.Atoi(match[1])

		// Get working directory of the process
		workdir := getWorkdir(pid)
		branch := getBranch(workdir)

		instances = append(instances, Instance{
			Port:    port,
			PID:     pid,
			Workdir: workdir,
			Branch:  branch,
		})
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Port < instances[j].Port
	})

	return instances
}

func getWorkdir(pid int) string {
	if runtime.GOOS == "darwin" {
		// macOS: use lsof -a -p PID -d cwd
		cmd := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn")
		output, err := cmd.Output()
		if err != nil {
			return "?"
		}
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "n") {
				return line[1:]
			}
		}
	} else {
		// Linux: read /proc/PID/cwd
		link, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
		if err == nil {
			return link
		}
	}
	return "?"
}

func getBranch(workdir string) string {
	if workdir == "?" {
		return "?"
	}
	cmd := exec.Command("git", "-C", workdir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(output))
}

func shortenPath(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func worktreeName(path string) string {
	// Check if this is a worktree by looking for "worktrees" in path
	if idx := strings.Index(path, "/worktrees/"); idx != -1 {
		parts := strings.Split(path[idx+11:], "/")
		return parts[0]
	}
	// Otherwise return the last directory component
	return filepath.Base(path)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printDashboard(instances []Instance) {
	clearScreen()
	fmt.Println("\033[1m╭────────────────────────────────────────────────────────────────╮\033[0m")
	fmt.Println("\033[1m│         🎮 Infinite Minesweeper Dev Dashboard                  │\033[0m")
	fmt.Println("\033[1m╰────────────────────────────────────────────────────────────────╯\033[0m")
	fmt.Println()

	if len(instances) == 0 {
		fmt.Println("  No running instances found on ports 8080-8099")
		fmt.Println()
		fmt.Println("  \033[2mStart one with: make go-run\033[0m")
	} else {
		fmt.Printf("  \033[1m%-6s %-20s %-8s %s\033[0m\n", "PORT", "WORKTREE", "PID", "BRANCH")
		fmt.Println("  " + strings.Repeat("─", 60))

		for i, inst := range instances {
			wt := worktreeName(inst.Workdir)
			fmt.Printf("  \033[36m%-6d\033[0m %-20s %-8d \033[33m%s\033[0m\n",
				inst.Port, wt, inst.PID, inst.Branch)
			if i < len(instances)-1 {
				fmt.Println()
			}
		}
	}

	fmt.Println()
	fmt.Println("\033[2m──────────────────────────────────────────────────────────────────\033[0m")
	fmt.Println("  \033[1mCommands:\033[0m")
	fmt.Println("    \033[36mo <port>\033[0m  Open in browser")
	fmt.Println("    \033[36mk <port>\033[0m  Kill instance")
	fmt.Println("    \033[36mr\033[0m         Refresh")
	fmt.Println("    \033[36mq\033[0m         Quit")
	fmt.Println()
	fmt.Print("  > ")
}

func openBrowser(port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		fmt.Println("  Cannot open browser on this OS")
		return
	}
	cmd.Start()
	fmt.Printf("  Opened %s\n", url)
}

func killInstance(port int, instances []Instance) {
	for _, inst := range instances {
		if inst.Port == port {
			if err := syscall.Kill(inst.PID, syscall.SIGTERM); err != nil {
				fmt.Printf("  Failed to kill PID %d: %v\n", inst.PID, err)
			} else {
				fmt.Printf("  Killed instance on port %d (PID %d)\n", port, inst.PID)
			}
			return
		}
	}
	fmt.Printf("  No instance found on port %d\n", port)
}

func main() {
	instances := findInstances()
	printDashboard(instances)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)

		if len(parts) == 0 {
			instances = findInstances()
			printDashboard(instances)
			continue
		}

		switch parts[0] {
		case "q", "quit", "exit":
			fmt.Println("  Bye!")
			return
		case "r", "refresh":
			instances = findInstances()
			printDashboard(instances)
		case "o", "open":
			if len(parts) < 2 {
				fmt.Print("  Port? > ")
				continue
			}
			port, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("  Invalid port")
			} else {
				openBrowser(port)
			}
			time.Sleep(500 * time.Millisecond)
			instances = findInstances()
			printDashboard(instances)
		case "k", "kill":
			if len(parts) < 2 {
				fmt.Print("  Port? > ")
				continue
			}
			port, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("  Invalid port")
			} else {
				killInstance(port, instances)
			}
			time.Sleep(500 * time.Millisecond)
			instances = findInstances()
			printDashboard(instances)
		default:
			// Try to parse as just a port number for quick open
			if port, err := strconv.Atoi(parts[0]); err == nil {
				openBrowser(port)
				time.Sleep(500 * time.Millisecond)
				instances = findInstances()
				printDashboard(instances)
			} else {
				fmt.Println("  Unknown command. Try: o <port>, k <port>, r, q")
				fmt.Print("  > ")
			}
		}
	}
}
