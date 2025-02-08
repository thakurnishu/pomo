package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const pidFile = "/tmp/tmuxstatus.pid"

// beep attempts to write the bell character to /dev/tty.
func beep() {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer tty.Close()
	tty.WriteString("\a")
}

// cleanup resets tmux's status-right option and removes the PID file.
func cleanup() {
	exec.Command("tmux", "set-option", "-g", "status-right", "").Run()
	os.Remove(pidFile)
}

// startPomodoro runs the pomodoro timer loop for the given duration.
func startPomodoro(duration time.Duration) {
	// Ensure we're inside a tmux session.
	if os.Getenv("TMUX") == "" {
		os.Exit(1)
	}

	// Write our PID to the PID file.
	pid := os.Getpid()
	err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
	if err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}

	// Set up a signal handler so that SIGINT/SIGTERM cleans up.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cleanup()
		os.Exit(0)
	}()

	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Update tmux's status-right every second.
	for {
		now := time.Now()
		if now.Before(endTime) {
			remaining := endTime.Sub(now).Truncate(time.Second)
			minutes := int(remaining.Minutes())
			seconds := int(remaining.Seconds()) % 60
			status := fmt.Sprintf("🍅 %02d:%02d", minutes, seconds)
			cmd := exec.Command("tmux", "set-option", "-g", "status-right", status)
			if err := cmd.Run(); err != nil {
				log.Printf("Error updating tmux status-right: %v", err)
			}
			time.Sleep(1 * time.Second)
		} else {
			// Timer has expired.
			elapsed := now.Sub(startTime).Truncate(time.Second)
			minutes := int(elapsed.Minutes())
			seconds := int(elapsed.Seconds()) % 60
			status := fmt.Sprintf("🍅 %02d:%02d passed", minutes, seconds)
			exec.Command("tmux", "set-option", "-g", "status-right", status).Run()

			// Emit a beep.
			beep()

			// Leave the finished status visible briefly.
			time.Sleep(5 * time.Second)
			cleanup()
			os.Exit(0)
		}
	}
}

// stopPomodoro stops a running pomodoro daemon by reading its PID file.
func stopPomodoro() {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		os.Exit(1)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		os.Exit(1)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Exit(1)
	}

	// Send SIGTERM to the process.
	proc.Signal(syscall.SIGTERM)
	os.Remove(pidFile)
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		// If already running, exit silently.
		if _, err := os.Stat(pidFile); err == nil {
			os.Exit(1)
		}

		// Use provided duration or default to 45 minutes.
		durationStr := "45m"
		if len(os.Args) >= 3 {
			durationStr = os.Args[2]
		}
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			os.Exit(1)
		}

		// If not in daemon mode, spawn a detached background process.
		if os.Getenv("TMUXSTATUS_DAEMON") == "" {
			cmd := exec.Command(os.Args[0], "start", durationStr)
			cmd.Env = append(os.Environ(), "TMUXSTATUS_DAEMON=1")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err != nil {
				log.Fatalf("Failed to start tmuxstatus in background: %v", err)
			}
			os.Exit(0)
		}
		// Daemon mode: run the pomodoro timer.
		startPomodoro(duration)

	case "stop":
		stopPomodoro()

	default:
		os.Exit(1)
	}
}
