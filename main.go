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
// It now supports pausing (via SIGUSR1) and resuming (via SIGUSR2).
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

	// Set up a signal channel to handle termination, pause, and resume.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Variables to handle pause/resume.
	paused := false
	var remaining time.Duration // remaining time when paused

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case s := <-sigChan:
			switch s {
			// Termination signals: cleanup and exit.
			case syscall.SIGINT, syscall.SIGTERM:
				cleanup()
				os.Exit(0)
			// SIGUSR1 pauses the timer.
			case syscall.SIGUSR1:
				if !paused {
					remaining = endTime.Sub(time.Now())
					paused = true
					status := fmt.Sprintf("🍅 PAUSED %02d:%02d", int(remaining.Minutes()), int(remaining.Seconds())%60)
					exec.Command("tmux", "set-option", "-g", "status-right", status).Run()
				}
			// SIGUSR2 resumes the timer.
			case syscall.SIGUSR2:
				if paused {
					endTime = time.Now().Add(remaining)
					paused = false
				}
			}
		case <-ticker.C:
			if paused {
				// When paused, keep showing the same remaining time.
				status := fmt.Sprintf("🍅 PAUSED %02d:%02d", int(remaining.Minutes()), int(remaining.Seconds())%60)
				exec.Command("tmux", "set-option", "-g", "status-right", status).Run()
			} else {
				now := time.Now()
				if now.Before(endTime) {
					rem := endTime.Sub(now).Truncate(time.Second)
					minutes := int(rem.Minutes())
					seconds := int(rem.Seconds()) % 60
					status := fmt.Sprintf("🍅 %02d:%02d", minutes, seconds)
					cmd := exec.Command("tmux", "set-option", "-g", "status-right", status)
					if err := cmd.Run(); err != nil {
						log.Printf("Error updating tmux status-right: %v", err)
					}
				} else {
					// Timer has expired.
					elapsed := time.Since(startTime).Truncate(time.Second)
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

// pausePomodoro sends the SIGUSR1 signal to the running pomodoro process.
func pausePomodoro() {
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

	// Send SIGUSR1 to pause the timer.
	proc.Signal(syscall.SIGUSR1)
}

// resumePomodoro sends the SIGUSR2 signal to the running pomodoro process.
func resumePomodoro() {
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

	// Send SIGUSR2 to resume the timer.
	proc.Signal(syscall.SIGUSR2)
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

	case "pause":
		pausePomodoro()

	case "resume":
		resumePomodoro()

	default:
		os.Exit(1)
	}
}
