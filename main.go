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
	if tk.Status == "deleted" {
		color.New(color.BgRed, color.FgWhite).Printf(" %12s ", "DELETED")
	} else {
		color.New(color.BgCyan).Printf(" %12s ", tk.Project)
	}
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
	'x': "delete task",
	'd': "mark done",
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
func printInfo(tk task, idx, total int) int {
	var cmd *exec.Cmd
	cmd = exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Println()
	printSummary(tk, idx, total)

	started, err := time.Parse(stamp, tk.Created)
	if err != nil {
		log.Fatal(err)
	}
	finished := time.Now()
	if len(tk.Completed) > 0 {
		finished, err = time.Parse(stamp, tk.Completed)
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println()
	if len(tk.Description) > 60 {
		fmt.Printf("Description:  %s\n", tk.Description)
	}
	fmt.Printf("Tags:        ")
	ntags := make([]string, 0, 10)
	for _, t := range tk.Tags {
		if isNormalTag(t) {
			ntags = append(ntags, t)
		}
	}
	for i, t := range ntags {
		color.New(color.FgRed+color.Attribute(i)).Printf(" %s", t)
	}
	fmt.Println()
	fmt.Printf("Started:      %s\n", started.Format(format))
	if len(tk.Completed) > 0 {
		now := time.Now().UTC()
		fmt.Printf("Completed:    %s [%vago]\n", finished.Format(format), age(now.Sub(finished)))
	}
	fmt.Printf("Age:          %v\n", age(finished.Sub(started)))
	fmt.Printf("UUID:         %s\n", tk.Uuid)
	fmt.Printf("XID:          %s\n", tk.Xid)
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
		return editDescription(tk)
	case 'a':
		return editAssigned(tk)
	case 'p':
		return editProject(tk)
	case 'c':
		return editTaskColor(tk)
	case 't':
		return editTags(tk)
	case 'r':
		return markReviewed(tk)
	case 'x':
		return deleteTask(tk)
	case 'd':
		return markDone(tk)
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

func printOptions(mp map[rune]string) {
	fmt.Println()
	var i color.Attribute
	for k, v := range mp {
		// color.New(color.FgRed+i).Printf("\t%q: %v\n", k, v)
		fmt.Printf("\t%q: %v\n", k, v)
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

func deleteTask(t task) int {
	t.Status = "deleted"
	doImport(t)
	return 1
}

func markDone(t task) int {
	t.Status = "completed"
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

// doImport imports the task.
func doImport(t task) {
	if len(t.Uuid) > 0 {
		// If the task gets externally modified, we'd end up blindly overwriting those changes.
		// So, run this check first for the mod time, and ensure that it's the same, before importing
		// the modified task.
		tasks, err := getTasks(t.Uuid)
		if err != nil {
			log.Fatalf("Error %v while retrieving tasks with UUID: %v", err, t.Uuid)
			return
		}
		if len(tasks) > 1 {
			log.Fatalf("Didn't expect to see more than 1 task with the same UUID: %v", t.Uuid)
		}
		if len(tasks) == 1 {
			prev := tasks[0]
			if prev.Modified != t.Modified {
				c := color.New(color.BgRed, color.FgWhite)
				c.Printf(
					"Task's mod time has changed [%q -> %q]. Please refresh before updating.",
					t.Modified, prev.Modified)
				fmt.Printf("\nPress enter to refresh.\n")
				r := make([]byte, 1)
				os.Stdin.Read(r)
				return
			}
		}
	}

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

var assigned = make(map[rune]string)
var allTags = make([]string, 0, 30)
var projects = make(map[rune]string)

func createMappingForAssigned(m map[rune]string, tags []string) {
	for _, t := range tags {
	CHARS:
		for i := 1; i < len(t); i++ { // iterate through characters.
			ch := rune(t[i])
			if _, has := m[ch]; !has {
				m[ch] = t
				break CHARS
			}
		}
	}
}

func generateMappings() {
	tasks, err := getTasks("")
	if err != nil {
		log.Fatal(err)
	}
	tags := make(map[string]bool)
	allProjects := make(map[string]bool)
	for _, t := range tasks {
		if len(t.Completed) > 0 || t.Status == "deleted" {
			continue
		}
		for _, tg := range t.Tags {
			tags[tg] = true
		}
		allProjects[t.Project] = true
	}

	for p := range allProjects {
		lp := strings.ToLower(p)
		for i := 0; i < len(lp); i++ {
			ch := rune(lp[i])
			if _, has := projects[ch]; !has {
				projects[ch] = p
				break
			}
		}
	}

	userTags := make([]string, 0, 10)
	for t := range tags {
		if len(t) == 0 {
			continue
		}

		if isNormalTag(t) {
			allTags = append(allTags, t)
		} else if t[0] == '@' {
			userTags = append(userTags, t)
		}
	}
	createMappingForAssigned(assigned, userTags)
}

func getTasks(filter string) ([]task, error) {
	var cmd *exec.Cmd
	var completed bool
	if len(filter) > 0 {
		args := strings.Split(filter, " ")
		args = append(args, "export")
		argf := args[:0]
		for _, arg := range args {
			if arg == "_end" {
				completed = true
				continue
			}
			argf = append(argf, arg)
		}
		cmd = exec.Command("task", argf...)
	} else {
		cmd = exec.Command("task", "export")
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var tasks []task
	err = json.Unmarshal(out.Bytes(), &tasks)
	final := tasks[:0]
	now := time.Now().UTC()

	for _, t := range tasks {
		if t.Status == "deleted" {
			continue
		}
		var end time.Time
		if len(t.Completed) > 0 {
			end, err = time.Parse(stamp, t.Completed)
			if err != nil {
				return tasks, err
			}
		}

		if completed {
			if now.Sub(end) < 7*24*time.Hour {
				final = append(final, t)
			}
		} else {
			if end.IsZero() {
				final = append(final, t)
			}
		}
	}
	return final, err
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

func showAndReviewTasks(tasks []task) {
	fmt.Println()
	now := time.Now().UTC()
	final := tasks[:0]

	for _, tk := range tasks {
		if len(tk.Reviewed) > 0 {
			rev, err := time.Parse(stamp, tk.Reviewed)
			if err == nil {
				if now.Sub(rev) < 24*time.Hour {
					continue
				}
			}
		}
		final = append(final, tk)
	}
	fmt.Printf("%d tasks already reviewed.\n", len(tasks)-len(final))
	tasks = final

	for i, tk := range tasks {
		if i >= 30 {
			break
		}
		printSummary(tk, i, len(tasks))
	}

	fmt.Println()
	if len(tasks) == 0 {
		fmt.Println("Found 0 tasks. Press anything to continue.")
		r := make([]byte, 1)
		os.Stdin.Read(r)
		return
	}

	fmt.Printf("Found %d tasks. Review (Y/n)? ", len(tasks))
	b := make([]byte, 1)
	os.Stdin.Read(b)
	if b[0] == 'n' || b[0] == 'q' {
		return
	}

	for i := 0; i < len(tasks); {
		if i < 0 || i >= len(tasks) {
			break
		}
		tk := tasks[i]
		move := printInfo(tk, i, len(tasks))
		tasks[i] = getTask(tk.Uuid) // refresh.
		i += move
	}
}

var help = map[rune]string{
	'q': "Quit",
	'c': "filter Clear",
	'd': "filter completeD",
	'a': "filter Assigned",
	'p': "filter Project",
	'n': "new task",
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
		args := strings.Split(filter, " ")
		final := args[:0]
		var found bool
		for _, arg := range args {
			if arg != "_end" {
				final = append(final, arg)
			} else {
				found = true
			}
		}
		if !found || len(final) == 0 {
			return ""
		}
		return strings.Join(final, " ")
	}

	if r[0] == 'd' {
		return filter + " _end"
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

	if r[0] == 'n' {
		args := strings.Split(filter, " ")
		var project, user string
		for _, arg := range args {
			if strings.HasPrefix(arg, "project:") {
				project = arg[8:]
			}
			if strings.HasPrefix(arg, "+@") {
				user = arg[1:]
			}
		}
		if len(project) == 0 {
			ch := showAndGetResponse("Project", projects)
			if a, ok := projects[ch]; ok {
				project = a
			} else {
				return filter
			}
		}
		if len(user) == 0 {
			ch := showAndGetResponse("Assign To", assigned)
			if a, ok := assigned[ch]; ok {
				user = a
			} else {
				return filter
			}
		}

		tags := []string{user, "green"}
		t := task{
			Project: project,
			Status:  "pending",
			Tags:    tags,
		}
		fmt.Println()
		editDescription(t)
		return filter
	}

	if r[0] == byte(10) { // Enter
		if len(filter) > 0 {
			uuids, err := getTasks(filter)
			if err != nil {
				log.Fatal(err)
			}
			showAndReviewTasks(uuids)
		}
		return filter
	}

	return filter
}

func main() {
	generateMappings()

	fmt.Println("Taskreview version 0.1")
	var filter string
	singleCharMode()
	for {
		filter = runShell(filter)
		filter = strings.Trim(filter, " \n")
	}
}
