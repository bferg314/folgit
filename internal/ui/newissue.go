package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/provider/github"
)

// issueForm writes new issues from the issues popup. After each one is
// created it clears and stays open for the next, like GitHub's "Create
// more". Closing it keeps the draft for the next time it's opened.
type issueForm struct {
	open    bool // shown in place of the list
	title   textinput.Model
	body    textarea.Model
	onBody  bool // body has focus, not title
	sending bool
	err     error
	created []github.Issue // made from this popup, newest first
}

type issueCreatedMsg struct {
	view  *issuesView
	issue github.Issue
	err   error
}

func (a *App) newIssueForm() *issueForm {
	f := &issueForm{title: textinput.New(), body: textarea.New()}
	f.title.Prompt = "│ "
	f.title.Placeholder = "Title"
	ts := f.title.Styles()
	ts.Focused.Prompt, ts.Blurred.Prompt = a.st.key, a.st.faintText
	ts.Focused.Text, ts.Blurred.Text = a.st.textS, a.st.textS
	ts.Focused.Placeholder, ts.Blurred.Placeholder = a.st.dim, a.st.dim
	ts.Cursor.Blink = false
	f.title.SetStyles(ts)

	f.body.Prompt = "│ "
	f.body.Placeholder = "Description (optional, Markdown)"
	f.body.ShowLineNumbers = false
	f.body.MaxHeight = 0
	bs := textarea.DefaultStyles(true)
	for _, s := range []*textarea.StyleState{&bs.Focused, &bs.Blurred} {
		*s = textarea.StyleState{Text: a.st.textS, Placeholder: a.st.dim, Prompt: a.st.faintText}
	}
	bs.Focused.Prompt = a.st.key
	bs.Cursor.Blink = false
	f.body.SetStyles(bs)
	return f
}

// openIssueForm shows the form, creating it on first use.
func (a *App) openIssueForm() tea.Cmd {
	v := a.issues
	switch {
	case v.loading && v.repo == "":
		return a.notify(0, "Wait for the issues to load first")
	case v.repo == "":
		return a.notify(2, "Can't create issues here: the repo's issues couldn't be loaded")
	}
	if v.form == nil {
		v.form = a.newIssueForm()
	}
	v.form.open = true
	return v.form.focus()
}

func (f *issueForm) focus() tea.Cmd {
	if f.onBody {
		f.title.Blur()
		return f.body.Focus()
	}
	f.body.Blur()
	return f.title.Focus()
}

func (a *App) issueFormKey(msg tea.KeyPressMsg) tea.Cmd {
	f := a.issues.form
	switch msg.String() {
	case "esc":
		f.open, f.err = false, nil
		return nil
	case "tab", "shift+tab":
		f.onBody = !f.onBody
		return f.focus()
	case "ctrl+s", "ctrl+enter":
		return a.submitIssue()
	case "enter":
		if !f.onBody {
			return a.submitIssue()
		}
	}
	if f.sending {
		return nil // the text is on its way; don't change it
	}
	return a.updateIssueForm(msg)
}

// updateIssueForm passes a message (a key, a paste, a cursor tick) to the
// focused field of an open form.
func (a *App) updateIssueForm(msg tea.Msg) tea.Cmd {
	if a.issues == nil || a.issues.form == nil || !a.issues.form.open {
		return nil
	}
	f := a.issues.form
	var cmd tea.Cmd
	if f.onBody {
		f.body, cmd = f.body.Update(msg)
	} else {
		f.title, cmd = f.title.Update(msg)
	}
	return cmd
}

func (a *App) submitIssue() tea.Cmd {
	v := a.issues
	f := v.form
	title := strings.TrimSpace(f.title.Value())
	switch {
	case f.sending:
		return nil
	case title == "":
		f.err = errTitleRequired
		f.onBody = false
		return f.focus()
	}
	f.sending, f.err = true, nil
	owner, name, _ := strings.Cut(v.repo, "/")
	body := strings.TrimSpace(f.body.Value())
	opt := a.cfg.GitHub
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		gh, err := githubClient(ctx, opt)
		if err != nil {
			return issueCreatedMsg{view: v, err: err}
		}
		is, err := gh.CreateIssue(ctx, owner, name, title, body)
		return issueCreatedMsg{view: v, issue: is, err: err}
	})
}

type formError string

func (e formError) Error() string { return string(e) }

const errTitleRequired = formError("An issue needs a title")

func (a *App) handleIssueCreated(msg issueCreatedMsg) tea.Cmd {
	v := a.issues
	if v == nil || v != msg.view || v.form == nil {
		return nil
	}
	f := v.form
	f.sending = false
	if msg.err != nil {
		f.err = msg.err
		return nil
	}
	is := msg.issue
	f.created = append([]github.Issue{is}, f.created...)
	v.issues = append([]github.Issue{is}, v.issues...)
	v.cursor, v.offset = 0, 0
	f.title.Reset()
	f.body.Reset()
	f.onBody = false
	return tea.Batch(f.focus(), a.notify(1, "Created #%d in %s", is.Number, v.repo))
}

// renderIssueForm fills the popup's list area (w by h cells).
func (a *App) renderIssueForm(w, h int) []string {
	st := a.st
	f := a.issues.form
	label := func(text string, on bool) string {
		if on {
			return st.boxTitle.Render(text)
		}
		return st.dim.Render(text)
	}

	f.title.SetWidth(max(10, w-3))
	// Title label and field, a blank line, the body label, then the body;
	// below it a blank line and a status line.
	bodyH := max(1, h-6)
	f.body.SetWidth(w)
	f.body.SetHeight(bodyH)

	lines := []string{
		label("Title", !f.onBody),
		fit(f.title.View(), w),
		"",
		label("Description", f.onBody),
	}
	lines = append(lines, strings.Split(f.body.View(), "\n")...)
	lines = append(lines, "")

	var status string
	switch {
	case f.sending:
		status = a.spin.View() + st.dim.Render(" Creating issue…")
	case f.err == errTitleRequired:
		status = st.warn.Render(f.err.Error())
	case f.err != nil:
		status = st.bad.Render("Couldn't create the issue: ") + st.dim.Render(f.err.Error())
	case len(f.created) > 0:
		var made []string
		for _, is := range f.created {
			made = append(made, st.warn.Render(fmt.Sprintf("#%d", is.Number))+" "+st.textS.Render(is.Title))
		}
		status = st.ok.Render("✓ Created ") + strings.Join(made, st.dim.Render(" · "))
	}
	lines = append(lines, fit(status, w))
	for i, l := range lines {
		lines[i] = fit(l, w)
	}
	return lines
}

// issueFormHint is the popup's bottom line while the form is open.
func (a *App) issueFormHint() string {
	st := a.st
	k := func(key, what string) string { return st.key.Render(key) + st.dim.Render(" "+what) }
	sep := st.dim.Render(" · ")
	if a.issues.form.onBody {
		return k("ctrl+s", "create") + sep + k("tab", "title") + sep + k("esc", "back to issues")
	}
	return k("enter", "create") + sep + k("tab", "description") + sep + k("esc", "back to issues")
}
