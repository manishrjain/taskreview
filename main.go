package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/manishrjain/keys"
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
	config    = flag.String("config", os.Getenv("HOME")+"/.taskreview",
		"Config path for key persistence.")
	cmdfilter = flag.String("f", "", "Filter specified in commandline.")
	short     *keys.Shortcuts
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
	Urgency     float64  `json:"urgency,omitempty"`
}

type ByUrgency []task

func (b ByUrgency) Len() int {
	return len(b)
}

func (b ByUrgency) Less(i int, j int) bool {
	return b[i].Urgency > b[j].Urgency
}

func (b ByUrgency) Swap(i int, j int) {
	b[i], b[j] = b[j], b[i]
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

	color.New(color.BgRed, color.FgWhite).Printf(" [%2d of %2d] %5.1f ", idx, total, tk.Urgency)
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

	short.Print("task", true)
	r := make([]byte, 1)
	os.Stdin.Read(r)

	ins, _ := short.MapsTo(rune(r[0]), "task")
	switch ins {
	case "back":
		return -1
	case "quit":
		return total
	case "description":
		return editDescription(tk)
	case "assigned":
		return editAssigned(tk)
	case "project":
		return editProject(tk)
	case "color":
		return editTaskColor(tk)
	case "tags":
		return editTags(tk)
	case "reviewed":
		return markReviewed(tk)
	case "delete":
		return deleteTask(tk)
	case "done":
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

func showAndGetResponse(header, label string) rune {
	if len(header) > 0 {
		color.New(color.BgRed, color.FgWhite).Printf(" %s: ", header)
	}
	short.Print(label, false)
	r := make([]byte, 1)
	os.Stdin.Read(r)
	return rune(r[0])
}

func editAssigned(t task) int {
	// We'll have to regenerate all the tags to modify the user tag.
	// Filter out user tag from existing tags.
	tags := t.Tags[:0]
	for _, t := range t.Tags {
		if t[0] != '@' {
			tags = append(tags, t)
		}
	}

	ch := showAndGetResponse("Assign To", "user")
	if a, ok := short.MapsTo(ch, "user"); ok {
		// Now add user tag into all tags.
		tags = append(tags, "@"+a)
	} else {
		return 0
	}
	t.Tags = tags
	doImport(t)
	return 0
}

func editProject(t task) int {
	ch := showAndGetResponse("Project", "project")
	if p, ok := short.MapsTo(ch, "project"); ok {
		t.Project = p
	} else {
		return 0
	}
	doImport(t)
	return 0
}

func editTags(t task) int {
	ch := showAndGetResponse("Tags", "tag")
	if tag, ok := short.MapsTo(ch, "tag"); ok {
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

func editTaskColor(t task) int {
	tags := t.Tags[:0]
	for _, tag := range t.Tags {
		if tag != "red" && tag != "green" && tag != "blue" {
			tags = append(tags, tag)
		}
	}

	ch := showAndGetResponse("Task Color", "color")
	if a, ok := short.MapsTo(ch, "color"); ok {
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
	sort.Sort(ByUrgency(final))
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

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	short.Print("help", true)
}

func runShell(filter string) string {
	clear()
	fmt.Println()
	color.New(color.BgBlue, color.FgWhite).Printf("task %s>", filter)

	r := make([]byte, 1)
	os.Stdin.Read(r)
	if r[0] == 10 { // Enter
		if len(filter) > 0 {
			uuids, err := getTasks(filter)
			if err != nil {
				log.Fatal(err)
			}
			showAndReviewTasks(uuids)
		}
		return filter
	}

	ins, _ := short.MapsTo(rune(r[0]), "help")
	switch ins {
	case "quit":
		return "-1"
	case "clear":
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
	case "completed":
		if strings.Index(filter, " _end") >= 0 {
			return filter
		}
		return filter + " _end"
	case "assigned":
		ch := showAndGetResponse("Assign To", "user")
		if a, ok := short.MapsTo(ch, "user"); ok {
			return filter + " +@" + a
		}
	case "project":
		ch := showAndGetResponse("Project", "project")
		if a, ok := short.MapsTo(ch, "project"); ok {
			return filter + " project:" + a
		}
	case "new":
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
			ch := showAndGetResponse("Project", "project")
			if a, ok := short.MapsTo(ch, "project"); ok {
				project = a
			} else {
				return filter
			}
		}
		if len(user) == 0 {
			ch := showAndGetResponse("Assign To", "user")
			if a, ok := short.MapsTo(ch, "user"); ok {
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
	default:
		return filter
	}
	return filter
}

func generateMappings() {
	tasks, err := getTasks("")
	if err != nil {
		log.Fatal(err)
	}
	for _, task := range tasks {
		if len(task.Completed) > 0 || task.Status == "deleted" {
			continue
		}
		short.AutoAssign(task.Project, "project")
		for _, t := range task.Tags {
			if len(t) == 0 {
				continue
			}

			if isNormalTag(t) {
				short.AutoAssign(t, "tag")
			} else if t[0] == '@' {
				short.AutoAssign(t[1:], "user")
			}
		} // end tags
	}

	short.BestEffortAssign('r', "red", "color")
	short.BestEffortAssign('b', "blue", "color")
	short.BestEffortAssign('g', "green", "color")

	short.BestEffortAssign('q', "quit", "help")
	short.BestEffortAssign('c', "clear", "help")
	short.BestEffortAssign('d', "completed", "help")
	short.BestEffortAssign('a', "assigned", "help")
	short.BestEffortAssign('p', "project", "help")
	short.BestEffortAssign('n', "new", "help")

	short.BestEffortAssign('e', "description", "task")
	short.BestEffortAssign('a', "assigned", "task")
	short.BestEffortAssign('p', "project", "task")
	short.BestEffortAssign('c', "color", "task")
	short.BestEffortAssign('t', "tags", "task")
	short.BestEffortAssign('r', "reviewed", "task")
	short.BestEffortAssign('b', "back", "task")
	short.BestEffortAssign('q', "quit", "task")
	short.BestEffortAssign('x', "delete", "task")
	short.BestEffortAssign('d', "done", "task")
}

func main() {
	flag.Parse()
	short = keys.ParseConfig(*config)
	generateMappings()

	fmt.Println("Taskreview version 0.1")
	filter := *cmdfilter
	singleCharMode()
	for {
		filter = runShell(filter)
		if filter == "-1" {
			break
		}
		filter = strings.Trim(filter, " \n")
	}
	short.Persist(*config)
}
