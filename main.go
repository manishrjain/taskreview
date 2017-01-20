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
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/manishrjain/keys"
)

const (
	stamp   = "20060102T150405Z"
	format  = "2006 Jan 02 Mon"
	URGENCY = iota
	DATE
	COLOR
)

var (
	uuidExp   *regexp.Regexp
	boldGreen *color.Color
	boldRed   *color.Color
	boldBlue  *color.Color
	config    = flag.String("config", os.Getenv("HOME")+"/.taskreview",
		"Config path for key persistence.")
	reviewTag = flag.String("rtag", "r:"+os.Getenv("USER"),
		"Tag to use for marking tasks as reviewed.")
	cmdfilter = flag.String("f", "", "Filter specified in commandline.")
	short     *keys.Shortcuts
	showAll   bool
	sortBy    = URGENCY
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
	pomo := color.New(color.BgBlack, color.FgWhite).PrintfFunc()

	ptag := tk.colorTag()
	user := tk.userTag()

	switch ptag {
	case "green":
		pomo = color.New(color.BgGreen, color.FgBlack).PrintfFunc()
	case "red":
		pomo = color.New(color.BgRed, color.FgWhite).PrintfFunc()
	case "blue":
		pomo = color.New(color.BgBlue, color.FgWhite).PrintfFunc()
	default:
		// pass
	}

	color.New(color.BgRed, color.FgWhite).Printf(" [%2d of %2d] ", idx, total)
	if tk.Status == "deleted" {
		color.New(color.BgRed, color.FgWhite).Printf(" X ")
	} else if tk.isDisputed() {
		color.New(color.BgRed, color.FgWhite).Printf(" D ")
	} else if tk.isReviewed() {
		color.New(color.BgGreen, color.FgBlack).Printf(" R ")
	} else {
		color.New(color.BgBlue, color.FgWhite).Printf(" N ")
	}
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
	if t == "red" || t == "green" || t == "blue" {
		return false
	}
	return true
}

// Returns back how much to move the index by.
func printInfo(tk task, idx, total int) int {
	clear()
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
		return tk.editDescription()
	case "assigned":
		return tk.editAssigned()
	case "project":
		return tk.editProject()
	case "color":
		return tk.editTaskColor()
	case "tags":
		return tk.editTags()
	case "reviewed":
		return tk.markReviewed()
	case "delete":
		return tk.deleteTask()
	case "done":
		return tk.markDone()
	case "disputed":
		return tk.toggleDisputed()
	default:
		return 1
	}
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

func getTasks(filter string) ([]task, error) {
	var cmd *exec.Cmd
	var completed int
	if len(filter) > 0 {
		args := strings.Split(filter, " ")
		args = append(args, "export")
		argf := args[:0]
		for _, arg := range args {
			if arg == "_end" {
				completed++
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

		if completed > 0 {
			if now.Sub(end) < time.Duration(completed)*7*24*time.Hour {
				final = append(final, t)
			}
		} else {
			if end.IsZero() {
				final = append(final, t)
			}
		}
	}
	sort.Sort(ByDefined(final))
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

func showAndReviewTasks(orig []task) {
	fmt.Println()
	var tasks []task

	for _, tk := range orig {
		if !showAll && tk.isReviewed() {
			continue
		}
		tasks = append(tasks, tk)
	}
	if !showAll {
		fmt.Printf("> %d tasks already reviewed.\n", len(orig)-len(tasks))
	} else {
		fmt.Println("> Showing all tasks.")
	}

SHOW:
	switch sortBy {
	case URGENCY:
		fmt.Println("> Sorted by Urgency.")
	case COLOR:
		fmt.Println("> Sorted by Color.")
	case DATE:
		fmt.Println("> Sorted by Date.")
	}
	fmt.Println()

	for i, tk := range tasks {
		if i >= 30 {
			break
		}
		printSummary(tk, i, len(tasks))
	}

	fmt.Printf("\nFound %d tasks.\n", len(tasks))
	short.Print("tasks", true)
	b := make([]byte, 1)
	os.Stdin.Read(b)
	if b[0] == 10 { // Enter
		return
	}

	var i int
	ins, _ := short.MapsTo(rune(b[0]), "tasks")
	switch ins {
	case "goto":
		i = getJump()
		if i == -1 {
			break
		}
		fallthrough
	case "review":
		for i < len(tasks) {
			if i < 0 || i >= len(tasks) {
				break
			}
			tk := tasks[i]
			move := printInfo(tk, i, len(tasks))
			tasks[i] = getTask(tk.Uuid) // refresh.
			i += move
		}
	case "toggle show all":
		showAll = !showAll
	case "fix":
		for i := 0; i < len(tasks); i++ {
			tk := &tasks[i]
			if len(tk.colorTag()) == 0 {
				fmt.Printf("Fixing task: %v\n", tk.Description)
				tk.Tags = append(tk.Tags, "green")
				tk.doImport()
			}
		}
		clear()
		goto SHOW
	case "sort by urgency":
		sortBy = URGENCY
		sort.Sort(ByDefined(tasks))
		clear()
		goto SHOW
	case "sort by date":
		sortBy = DATE
		sort.Sort(ByDefined(tasks))
		clear()
		goto SHOW
	case "sort by color":
		sortBy = COLOR
		sort.Sort(ByDefined(tasks))
		clear()
		goto SHOW
	}
}

func getJump() int {
	lineInputMode()
	defer singleCharMode()

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Jump to: ")
	jump, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	j, err := strconv.Atoi(jump[:len(jump)-1])
	if err != nil {
		return -1
	}
	return j
}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	fmt.Println()
}

func searchTerms() string {
	fmt.Println()
	lineInputMode()
	defer singleCharMode()

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Enter search terms: ")
	desc, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return strings.Trim(desc, " \n")
}

func runShell(filter string) string {
	clear()
	short.Print("help", true)
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
		return ""
	case "completed":
		return filter + " _end"
	case "search":
		terms := searchTerms()
		return filter + " " + terms

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
	case "tag":
		ch := showAndGetResponse("Tag", "tag")
		if a, ok := short.MapsTo(ch, "tag"); ok {
			return filter + " +" + a
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
				user = "@" + a
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
		t.editDescription()
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
	short.BestEffortAssign('t', "tag", "help")
	short.BestEffortAssign('s', "search", "help")

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
	short.BestEffortAssign('i', "disputed", "task")

	short.BestEffortAssign('f', "fix", "tasks")
	short.BestEffortAssign('a', "toggle show all", "tasks")
	short.BestEffortAssign('r', "review", "tasks")
	short.BestEffortAssign('u', "sort by urgency", "tasks")
	short.BestEffortAssign('d', "sort by date", "tasks")
	short.BestEffortAssign('c', "sort by color", "tasks")
	short.BestEffortAssign('g', "goto", "tasks")
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
