package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
)

type AppState struct {
	config *Config

	screen      tcell.Screen
	screenDirty bool

	workingDir  string
	dirContents []string
	dirMutex    sync.RWMutex
	selectedDir int

	currentKeySequence []string
	sequenceClearTimer *time.Timer

	lastMessage  *string
	messageMutex sync.Mutex
	messageTimer *time.Timer

	eventCh                   chan tcell.Event
	screenRefreshRequest      chan struct{}
	dirContentsRefreshRequest chan struct{}
	quit                      chan struct{}
}

func main() {
	var err error
	state := &AppState{
		screenDirty: true, // instant draw when the app starts

		selectedDir: 0,

		lastMessage: new(string),

		eventCh:                   make(chan tcell.Event),
		screenRefreshRequest:      make(chan struct{}, 1),
		dirContentsRefreshRequest: make(chan struct{}),
		quit:                      make(chan struct{}),
	}

	state.screen, err = tcell.NewScreen()
	if err != nil {
		fatalError(nil, "making a new screen", err)
	}
	// start the app
	err = state.screen.Init()
	if err != nil {
		fatalError(nil, "initiating the screen", err)
	}
	defer state.screen.Fini()

	state.workingDir, err = os.Getwd()
	if err != nil {
		fatalError(nil, "getting working dir", err)
	}

	state.config, err = loadConfig()
	if err != nil {
		fatalError(nil, "loading config", err)
	}

	// initialize the dir contents instantly
	entries, err := os.ReadDir(state.workingDir)
	if err != nil {
		fatalError(state.screen, "initializing contents", err)
	}
	state.dirMutex.Lock()
	state.dirContents = make([]string, len(entries))
	for i, e := range entries {
		state.dirContents[i] = e.Name()
	}
	state.dirMutex.Unlock()

	go state.dirContentsUpdater()
	go state.eventPoller()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// main loop
	for {
		select {
		case <-ticker.C:
			state.draw()
		case event := <-state.eventCh:
			state.handleEvent(event)
		case <-state.screenRefreshRequest:
			state.draw()
		case <-state.quit:
			return
		}
	}
}

func (a *AppState) draw() {
	if !a.screenDirty {
		return
	}
	a.screen.Clear()
	_, height := a.screen.Size()
	maxRows := height - 1

	putString(a.screen, ""+a.workingDir, 1, 0, tcell.StyleDefault)
	putString(a.screen, *a.lastMessage, 1, maxRows, tcell.StyleDefault)

	a.dirMutex.Lock()
	defer a.dirMutex.Unlock()

	// TODO: scrolloff logic
	for i := 0; i < len(a.dirContents) && i < maxRows; i++ {
		var style tcell.Style
		if a.selectedDir == i {
			style = tcell.StyleDefault.Reverse(true)
		} else {
			style = tcell.StyleDefault
		}
		putString(a.screen, a.dirContents[i], 1, 1+i, style)
	}

	a.screen.Show()
	a.screenDirty = false
}

// rechecks the contents of the current working directory every couple seconds as to
// not show outdated data (probably should redo this with some fs watcher thing?)
func (a *AppState) dirContentsUpdater() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	update := func() {
		entries, err := os.ReadDir(a.workingDir)
		if err != nil {
			fatalError(a.screen, "updating dir", err)
		}

		a.dirMutex.Lock()
		defer a.dirMutex.Unlock()
		a.dirContents = make([]string, len(entries))
		for i, e := range entries {
			a.dirContents[i] = e.Name()
		}

		a.requestUpdate(a.screenRefreshRequest)
	}

	for {
		select {
		case <-ticker.C:
			update()
		case <-a.dirContentsRefreshRequest:
			update()
		case <-a.quit:
			return
		}
	}
}
func (a *AppState) eventPoller() {
	for {
		event := a.screen.PollEvent()
		select {
		case a.eventCh <- event: // send to main loop
		case <-a.quit:
			return
		}
	}
}

func (a *AppState) handleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		// keybind handling
		var key string
		if ev.Key() == tcell.KeyRune {
			key = strings.ToLower(string(ev.Rune()))
		} else {
			key = strings.ToLower(ev.Name())
		}

		action, exists := a.config.AbsoluteKeybinds[key]
		// check if action exists and hasnt been overriden
		if exists && action != "" {
			a.formattedCommand(action)
			return
		}

		if a.currentKeySequence != nil {
			key = "+" + key
		}
		a.currentKeySequence = append(a.currentKeySequence, key)

		if a.sequenceClearTimer != nil {
			a.sequenceClearTimer.Stop() // if there a timer running, stop it
		}
		a.sequenceClearTimer = time.AfterFunc( // make a new timer
			time.Duration(a.config.Options.KeybindDuration)*time.Millisecond,
			func() {
				a.currentKeySequence = nil
			},
		)

		action, exists = a.config.Keybinds[strings.Join(a.currentKeySequence, "+")]
		if exists {
			a.currentKeySequence = nil
			a.formattedCommand(action)
		}

		a.setMessage(strings.Join(a.currentKeySequence, ""), 800)
	case *tcell.EventResize:
		a.screen.Sync()
		a.draw()
	}
}
func (a *AppState) executeBuiltinKeybind(action string) {
	switch action {
	case "quit":
		close(a.quit)
	case "select_up":
		if a.selectedDir == 0 {
			a.selectedDir = len(a.dirContents) - 1
		} else {
			a.selectedDir -= 1
		}
		a.requestUpdate(a.screenRefreshRequest)
	case "select_down":
		if a.selectedDir == len(a.dirContents)-1 {
			a.selectedDir = 0
		} else {
			a.selectedDir += 1
		}
		a.requestUpdate(a.screenRefreshRequest)
	case "dir_forwards":
		selectedDir := a.dirContents[a.selectedDir]
		newWorkingDir := filepath.Join(a.workingDir, selectedDir)

		info, err := os.Stat(newWorkingDir)
		if os.IsNotExist(err) {
			fatalError(a.screen, "selecting the new directory", fmt.Errorf("no such directory: `%v`", newWorkingDir))
		} else if err != nil {
			fatalError(a.screen, "selecting the new directory", err)
		}

		if !info.IsDir() {
			a.setMessage("not a directory", 1500)
			return
		}

		a.workingDir = newWorkingDir
		a.selectedDir = 0
		a.requestUpdate(a.dirContentsRefreshRequest)
	case "dir_backwards":
		a.workingDir = filepath.Dir(a.workingDir)
		a.selectedDir = 0
		a.requestUpdate(a.dirContentsRefreshRequest)
	case "clear_key_sequence":
		a.currentKeySequence = nil
		if a.sequenceClearTimer != nil {
			a.sequenceClearTimer.Stop()
		}
		a.clearMessage()
	case "quit_cd":
		absPath, err := filepath.Abs(a.workingDir)
		if err != nil {
			fatalError(a.screen, "getting absolute path", err)
		}

		close(a.quit) // quit the app so the last thing printed is the cd command
		// this will only work if the app is ran with `eval $(fe)`
		fmt.Printf("cd %q\n", absPath)

	default:
		fatalError(a.screen, "executing builtin keybind", fmt.Errorf("action `%v` does not exist", action))
	}
}

func (a *AppState) setMessage(msg string, duration time.Duration) {
	a.messageMutex.Lock()
	defer a.messageMutex.Unlock()

	if a.messageTimer != nil {
		a.messageTimer.Stop()
	}

	*a.lastMessage = msg
	a.requestUpdate(a.screenRefreshRequest)

	// TODO: move duration to config
	a.messageTimer = time.AfterFunc(duration*time.Millisecond, func() {
		a.clearMessage()
	})
}
func (a *AppState) clearMessage() {
	a.messageMutex.Lock()
	defer a.messageMutex.Unlock()

	*a.lastMessage = ""
	a.requestUpdate(a.screenRefreshRequest)
	a.messageTimer = nil
}

func (a *AppState) requestUpdate(requestChannel chan struct{}) {
	a.screenDirty = true
	select {
	case requestChannel <- struct{}{}:
	default: // prevent blocking if already queued
	}
}

func (a *AppState) formattedCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		fatalError(
			a.screen,
			"executing formatted shell command",
			fmt.Errorf("empty command"),
		)
	}

	formattedCommand, appCommands, err := namedFormatter(
		command,
		a.makeFormattingData(),
	)
	if err != nil {
		fatalError(a.screen, "formatting a custom command", err)
	}

	cmd := exec.Command(formattedCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		a.setMessage(err.Error(), 2500)
	}

	// do the builtin commands after the shell command
	for _, appCommand := range appCommands {
		a.executeBuiltinKeybind(appCommand)
	}

	return string(output)
}

func fatalError(screen tcell.Screen, where string, err error) {
	if screen != nil {
		screen.Fini() // exit the screen
	}
	fmt.Printf("fe: failed at %v:\n%v\n\n", where, err.Error())
	os.Exit(1)
}
func putString(screen tcell.Screen, str string, x, y int, style tcell.Style) {
	for i, char := range str {
		screen.SetContent(x+i, y, char, nil, style)
	}
}
