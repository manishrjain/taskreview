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
	"github.com/pkg/errors"
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
	Reviewed    string   `json:"reviewed,omitempty"`
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

func getTask(uuid string) task {
	cmd := exec.Command("task", uuid, "export")
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
	return task
}

func printSummary(tk task, idx, total int) {
	var user, ptag string
	pomo := color.New(color.BgBlack, color.FgWhite).PrintfFunc()

	for _, tag := range tk.Tags {
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
			// pass
		}
	}

	color.New(color.BgRed, color.FgWhite).Printf(" [%2d of %2d] ", idx, total)
	color.New(color.BgYellow, color.FgBlack).Printf(" %13s ", user)
	color.New(color.BgCyan).Printf(" %12s ", tk.Project)
	desc := tk.Description
	if len(desc) > 60 {
		desc = desc[:60]
	}
	color.New(color.BgWhite, color.FgBlack).Printf(" %-60s", desc)
	pomo(" %-10v ", ptag)
	fmt.Println()
}

var taskHelp = map[rune]string{
	'e': "edit description",
	'a': "edit assigned",
	'p': "edit project",
	'c': "edit color",
	't': "edit tags",
	'r': "mark reviewed",
	'b': "go back",
	'q': "quit",
}

func isNormalTag(t string) bool {
	if len(t) == 0 {
		return false
	}
	if t[0] >= 'A' && t[0] <= 'Z' {
		return false
	}
	if t[0] == '@' || t[0] == '-' {
		return false
	}
	if t == "red" || t == "green" || t == "blue" || t == "black" {
		return false
	}
	return true
}

// Returns back how much to move the index by.
func printInfo(uuid string, idx, total int) int {
	var cmd *exec.Cmd
	cmd = exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Println()
	task := getTask(uuid)
	printSummary(task, idx, total)

	started, err := time.Parse(stamp, task.Created)
	if err != nil {
		log.Fatal(err)
	}
	finished := time.Now()
	if len(task.Completed) > 0 {
		finished, err = time.Parse(stamp, task.Completed)
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println()
	if len(task.Description) > 60 {
		fmt.Printf("Description:  %s\n", task.Description)
	}
	fmt.Printf("Tags:        ")
	ntags := make([]string, 0, 10)
	for _, t := range task.Tags {
		if isNormalTag(t) {
			ntags = append(ntags, t)
		}
	}
	for i, t := range ntags {
		color.New(color.FgRed+color.Attribute(i)).Printf(" %s", t)
	}
	fmt.Println()
	fmt.Printf("Started:      %s\n", started.Format(format))
	if len(task.Completed) > 0 {
		fmt.Printf("Completed:    %s\n", finished.Format(format))
	}
	fmt.Printf("Age:          %v\n", age(finished.Sub(started)))
	fmt.Printf("UUID:         %s\n", task.Uuid)
	fmt.Println()

	printOptions(taskHelp)
	r := make([]byte, 1)
	os.Stdin.Read(r)

	switch r[0] {
	case 'b':
		return -1
	case 'q':
		return total
	case 'e':
		return editDescription(task)
	case 'a':
		return editAssigned(task)
	case 'p':
		return editProject(task)
	case 'c':
		return editTaskColor(task)
	case 't':
		return editTags(task)
	case 'r':
		return markReviewed(task)
	default:
		return 1
	}
}

func editDescription(t task) int {
	lineInputMode()
	defer singleCharMode()

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Enter description: ")
	desc, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	t.Description = strings.Trim(desc, " \n")
	if len(t.Description) > 0 {
		doImport(t)
	}
	return 0
}

var assigned = map[rune]string{
	'a': "@ashwin",
	'j': "@jchiu",
	'm': "@manish",
	'p': "@pawan",
	'o': "@porwaladisha",
}

var projects = map[rune]string{
	'd': "Development",
	't': "Technical",
	'c': "Company",
	'l': "Learning",
	'n': "Design",
}

func printOptions(mp map[rune]string) {
	fmt.Println()
	var i color.Attribute
	for k, v := range mp {
		color.New(color.FgRed+i).Printf("\t%q: %v\n", k, v)
		i++
		i = i % 6
	}
	fmt.Println()
}

func showAndGetResponse(label string, m map[rune]string) rune {
	color.New(color.BgRed, color.FgWhite).Printf(" %s: ", label)
	printOptions(m)
	r := make([]byte, 1)
	os.Stdin.Read(r)
	return rune(r[0])
}

func editAssigned(t task) int {
	tags := t.Tags[:0]
	for _, t := range t.Tags {
		if t[0] != '@' {
			tags = append(tags, t)
		}
	}

	ch := showAndGetResponse("Assign To", assigned)
	if a, ok := assigned[ch]; ok {
		tags = append(tags, a)
	} else {
		return 0
	}
	t.Tags = tags
	doImport(t)
	return 0
}

func editProject(t task) int {
	ch := showAndGetResponse("Project", projects)
	if p, ok := projects[ch]; ok {
		t.Project = p
	} else {
		return 0
	}
	doImport(t)
	return 0
}

func editTags(t task) int {
	m := make(map[rune]string)
	for _, t := range allTags {
	CHARS:
		for i := 0; i < len(t); i++ { // iterate through characters.
			ch := rune(t[i])
			if _, has := m[ch]; !has {
				m[ch] = t
				break CHARS
			}
		}
	}
	ch := showAndGetResponse("Tags", m)
	if tag, ok := m[ch]; ok {
		newt := t.Tags[:0]
		found := false
		for _, prev := range t.Tags {
			if prev != tag {
				newt = append(newt, prev)
			} else {
				found = true
			}
		}
		if !found {
			newt = append(newt, tag)
		}
		t.Tags = newt
		doImport(t)
	}
	return 0
}

func markReviewed(t task) int {
	t.Reviewed = time.Now().UTC().Format(stamp)
	doImport(t)
	return 1
}

var taskColors = map[rune]string{
	'r': "red",
	'b': "blue",
	'g': "green",
}

func editTaskColor(t task) int {
	color.New(color.BgRed, color.FgWhite).Printf(" Set Task Color: ")
	printOptions(taskColors)
	tags := t.Tags[:0]
	for _, tag := range t.Tags {
		if tag != "red" && tag != "green" && tag != "blue" {
			tags = append(tags, tag)
		}
	}

	r := make([]byte, 1)
	os.Stdin.Read(r)
	ch := rune(r[0])
	if a, ok := taskColors[ch]; ok {
		tags = append(tags, a)
	} else {
		return 0
	}
	t.Tags = tags
	doImport(t)
	return 0
}

// doImport imports the task and returns it's UUID and error.
func doImport(t task) {
	body, err := json.Marshal(t)
	if err != nil {
		log.Fatalf("While importing: %v", err)
	}

	cmd := fmt.Sprintf("echo -n %q | task import", body)
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Fatal(errors.Wrapf(err, "doImport [%v] out:%q", cmd, out))
	}
}

func parseUuids(out bytes.Buffer) ([]string, error) {
	var tasks []task
	if err := json.Unmarshal(out.Bytes(), &tasks); err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if len(t.Completed) > 0 {
			continue
		}
		uuids = append(uuids, t.Uuid)
	}
	return uuids, nil
}

var allTags = make([]string, 0, 30)

func cacheAllTags() {
	cmd := exec.Command("task", "tags")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Fatalf("getAllTags: %v", err)
	}
	s := bufio.NewScanner(&out)
	for s.Scan() {
		line := s.Text()
		if len(line) == 0 {
			continue
		}
		t := strings.Split(line, " ")[0]

		if !isNormalTag(t) {
			continue
		}
		if t[0] == '_' {
			continue
		}
		allTags = append(allTags, t)
	}
}

func getTasks(filter string) ([]string, error) {
	var cmd *exec.Cmd
	if len(filter) > 0 {
		args := strings.Split(filter, " ")
		args = append(args, "export")
		cmd = exec.Command("task", args...)
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

func singleCharMode() {
	// disable input buffering
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
	// do not display entered characters on the screen
	exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
}

func lineInputMode() {
	exec.Command("stty", "-F", "/dev/tty", "cooked").Run()
	exec.Command("stty", "-F", "/dev/tty", "echo").Run()
}

func showAndReviewTasks(uuids []string) {
	fmt.Println()
	now := time.Now().UTC()
	final := uuids[:0]

	for i := 0; i < len(uuids); i++ {
		tk := getTask(uuids[i])
		if len(tk.Reviewed) > 0 {
			rev, err := time.Parse(stamp, tk.Reviewed)
			if err == nil {
				if now.Sub(rev) < 24*time.Hour {
					continue
				}
			}
		}
		final = append(final, uuids[i])
	}
	fmt.Printf("%d tasks already reviewed.\n", len(uuids)-len(final))
	uuids = final

	for i, uuid := range uuids {
		if i >= 30 {
			break
		}
		tk := getTask(uuid)
		printSummary(tk, i, len(uuids))
	}

	fmt.Println()
	if len(uuids) == 0 {
		fmt.Println("Found 0 tasks.")
		time.Sleep(3 * time.Second)
		return
	}

	fmt.Printf("Found %d tasks. Review (Y/n)? ", len(uuids))
	b := make([]byte, 1)
	os.Stdin.Read(b)
	if b[0] == 'n' {
		return
	}

	for i := 0; i < len(uuids); {
		if i < 0 || i >= len(uuids) {
			break
		}
		uuid := uuids[i]
		i += printInfo(uuid, i, len(uuids))
	}
}

var help = map[rune]string{
	'q': "Quit",
	'c': "filter Clear",
	'd': "filter completeD",
	'a': "filter Assigned",
	'p': "filter Project",
}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	printOptions(help)
}

func runShell(filter string) string {
	clear()
	color.New(color.BgBlue, color.FgWhite).Printf("\ntask %s>", filter)

	r := make([]byte, 1)
	if _, err := os.Stdin.Read(r); err != nil {
		log.Fatal(err)
	}

	if r[0] == 'q' {
		os.Exit(0)
	}

	if r[0] == 'c' {
		return ""
	}

	if r[0] == 'd' {
		uuids, err := getCompletedTasks()
		if err != nil {
			log.Fatal(err)
		}
		showAndReviewTasks(uuids)
		return ""
	}

	if r[0] == 'a' {
		ch := showAndGetResponse("Assign To", assigned)
		if a, ok := assigned[ch]; ok {
			return filter + " +" + a
		}
	}

	if r[0] == 'p' {
		ch := showAndGetResponse("Project", projects)
		if a, ok := projects[ch]; ok {
			return filter + " project:" + a
		}
	}

	if r[0] == byte(10) { // Enter
		if len(filter) > 0 {
			uuids, err := getTasks(filter)
			if err != nil {
				log.Fatal(err)
			}
			showAndReviewTasks(uuids)
		}
		return ""
	}

	return filter
}

func main() {
	cacheAllTags()

	fmt.Println("Taskreview version 0.1")
	var filter string
	singleCharMode()
	for {
		filter = runShell(filter)
		filter = strings.Trim(filter, " \n")
	}
}
