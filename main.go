package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	stamp  = "20060102T150405Z"
	format = "2006 Jan 02 Mon"
)

var (
	uuidExp   *regexp.Regexp
	boldGreen *color.Color
	boldRed   *color.Color
	boldBlue  *color.Color
)

func init() {
	var err error
	uuidExp, err = regexp.Compile("([0-9a-f]{8})")
	if err != nil {
		log.Fatal(err)
	}
	boldGreen = color.New(color.FgGreen).Add(color.Bold)
	boldRed = color.New(color.FgRed).Add(color.Bold)
	boldBlue = color.New(color.FgBlue).Add(color.Bold)
}

type task struct {
	Completed   string   `json:"end,omitempty"`
	Created     string   `json:"entry,omitempty"`
	Description string   `json:"description,omitempty"`
	Modified    string   `json:"modified,omitempty"`
	Project     string   `json:"project,omitempty"`
	Status      string   `json:"status,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Uuid        string   `json:"uuid,omitempty"`
	Xid         string   `json:"xid,omitempty"`
}

func age(dur time.Duration) string {
	var res string
	if dur > 24*time.Hour {
		days := dur / (24 * time.Hour)
		res += fmt.Sprintf("%d days ", days)
		dur -= days * 24 * time.Hour
	}

	if dur > time.Hour {
		res += fmt.Sprintf("%d hours ", int(dur.Hours()))
	}
	if dur < time.Hour {
		res += fmt.Sprintf("%d mins ", int(dur.Minutes()))
	}

	return res
}

// Returns back how much to move the index by.
func printInfo(uuid string, idx, total int) int {
	var cmd *exec.Cmd
	cmd = exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	fmt.Println()

	cmd = exec.Command("task", uuid, "export")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	var tasks []task
	if err := json.Unmarshal(out.Bytes(), &tasks); err != nil {
		log.Fatal(err)
	}
	if len(tasks) != 1 {
		log.Fatalf("Expected exactly one task for: %v", uuid)
	}
	task := tasks[0]

	var user, ptag string
	pomo := color.New(color.BgBlack, color.FgWhite).PrintfFunc()

	rem := make([]string, 0, 10)
	for _, tag := range task.Tags {
		switch {
		case tag == "green":
			ptag = tag
			pomo = color.New(color.BgGreen, color.FgBlack).PrintfFunc()
		case tag == "red":
			ptag = tag
			pomo = color.New(color.BgRed, color.FgWhite).PrintfFunc()
		case tag == "blue":
			ptag = tag
			pomo = color.New(color.BgBlue, color.FgWhite).PrintfFunc()
		case strings.HasPrefix(tag, "@"):
			user = tag
		default:
			rem = append(rem, tag)
		}
	}
	started, err := time.Parse(stamp, task.Created)
	if err != nil {
		log.Fatal(err)
	}
	finished, err := time.Parse(stamp, task.Completed)
	if err != nil {
		log.Fatal(err)
	}

	color.New(color.BgRed, color.FgWhite).Printf(" [%d of %d] ", idx, total)
	color.New(color.BgYellow, color.FgBlack).Printf(" %7s ", user)
	color.New(color.BgWhite, color.FgBlack).Printf(" %-60s", task.Description)
	pomo(" %-10v ", ptag)
	fmt.Println()
	fmt.Println()
	fmt.Printf("Project:      ")
	color.New(color.FgYellow).Printf("%s\n", task.Project)
	fmt.Printf("Tags:         %s\n", strings.Join(rem, " "))
	fmt.Printf("Started:      %s\n", started.Format(format))
	fmt.Printf("Completed:    %s\n", finished.Format(format))
	fmt.Printf("Age:          %v\n", age(finished.Sub(started)))
	fmt.Printf("UUID:         %s\n", task.Uuid)
	fmt.Println()

	fmt.Println(`
	Press ENTER to mark reviewed, s to skip
	Press r for red task, b for blue task, g for green task
	Press w to toggle _WaitingFor
	Press d to set project:Development, t to set project:Technical
	Press b to go back to the last task
	`)
	r := make([]byte, 1)
	os.Stdin.Read(r)
	if r[0] == 'b' {
		return -1
	}
	return 1
}

func parseUuids(out bytes.Buffer) ([]string, error) {
	var tasks []task
	if err := json.Unmarshal(out.Bytes(), &tasks); err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		uuids = append(uuids, t.Uuid)
	}
	return uuids, nil
}

func getTasks(filter string) ([]string, error) {
	var cmd *exec.Cmd
	if len(filter) > 0 {
		cmd = exec.Command("task", filter, "export")
	} else {
		cmd = exec.Command("task", "export")
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return parseUuids(out)
}

func getCompletedTasks() ([]string, error) {
	cmd := exec.Command("task", "completed", "end.after:today-1wk")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(&out)
	uuids := make([]string, 0, 10)
	for s.Scan() {
		uuid := uuidExp.FindString(s.Text())
		if len(uuid) == 0 {
			continue
		}
		uuids = append(uuids, uuid)
	}
	return uuids, nil
}

func main() {
	// disable input buffering
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
	// do not display entered characters on the screen
	exec.Command("stty", "-F", "/dev/tty", "-echo").Run()

	uuids, err := getCompletedTasks()
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < len(uuids); {
		if i < 0 || i >= len(uuids) {
			break
		}
		uuid := uuids[i]
		i += printInfo(uuid, i, len(uuids))
	}
}
