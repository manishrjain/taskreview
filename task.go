package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
)

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

type ByDefined []task

func (b ByDefined) Len() int          { return len(b) }
func (b ByDefined) Swap(i int, j int) { b[i], b[j] = b[j], b[i] }
func (b ByDefined) Less(i int, j int) bool {
	if sortBy == URGENCY {
		return b[i].Urgency > b[j].Urgency

	} else if sortBy == DATE {
		t1 := b[i].sortTime()
		t2 := b[j].sortTime()
		return t2.Before(t1)
	} else if sortBy == COLOR {
		t1 := b[i].sortColor()
		t2 := b[j].sortColor()
		return t1 < t2
	}

	log.Fatalf("Unhandled sortBy case for: %v", sortBy)
	return true
}

func (tk task) sortTime() time.Time {
	ts := tk.Completed
	if len(ts) == 0 {
		ts = tk.Created
	}
	t, err := time.Parse(stamp, ts)
	if err != nil {
		log.Fatalf("While trying to parse: %v. Got err: %v", ts, err)
	}
	return t
}

func (tk task) sortColor() int {
	c := tk.colorTag()
	switch c {
	case "red":
		return 0
	case "blue":
		return 1
	case "green":
		return 2
	default:
		return -1
	}
}

func (tk task) colorTag() string {
	for _, t := range tk.Tags {
		if t == "green" || t == "blue" || t == "red" {
			return t
		}
	}
	return ""
}

func (tk task) userTag() string {
	for _, t := range tk.Tags {
		if strings.HasPrefix(t, "@") {
			return t
		}
	}
	return ""
}

func (tk task) isReviewed() bool {
	now := time.Now().UTC()
	if len(tk.Completed) == 0 {
		// Incomplete task. So, only update local version.
		if len(tk.Reviewed) > 0 {
			rev, err := time.Parse(stamp, tk.Reviewed)
			if err == nil {
				if now.Sub(rev) < 24*time.Hour {
					return true
				}
			}
		}
	} else {
		// Task has been completed. So, add a reviewed tag.
		for _, t := range tk.Tags {
			if t == *reviewTag {
				return true
			}
		}
	}
	return false
}

var kDisputed string = "disputed"

func (tk task) isDisputed() bool {
	for _, t := range tk.Tags {
		if t == kDisputed {
			return true
		}
	}
	return false
}

func (tk task) markDisputed() int {
	if tk.isDisputed() {
		return 1
	}
	tk.Tags = append(tk.Tags, kDisputed)
	tk.doImport()
	return 1
}

func (t task) markDone() int {
	t.Status = "completed"
	t.doImport()
	return 1
}

func (t task) markReviewed() int {
	if t.isReviewed() {
		return 1
	}
	if len(t.Completed) == 0 {
		t.Reviewed = time.Now().UTC().Format(stamp)
	} else {
		t.Tags = append(t.Tags, *reviewTag)
	}
	t.doImport()
	return 1
}

func (t task) editTaskColor() int {
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
	t.doImport()
	return 0
}

func (t task) editDescription() int {
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
		t.doImport()
	}
	return 0
}

func (t task) editAssigned() int {
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
	t.doImport()
	return 0
}

func (t task) editProject() int {
	ch := showAndGetResponse("Project", "project")
	if p, ok := short.MapsTo(ch, "project"); ok {
		t.Project = p
	} else {
		return 0
	}
	t.doImport()
	return 0
}

func (t task) editTags() int {
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
		t.doImport()
	}
	return 0
}

func (t task) deleteTask() int {
	t.Status = "deleted"
	t.doImport()
	return 1
}

// doImport iports the task.
func (t task) doImport() {
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
		log.Fatal(errors.Wrapf(err, "doImport [v] out:%q", cmd, out))
	}
}
