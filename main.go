package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func main() {
	myApp := app.New()
	myApp.Settings().SetTheme(theme.DarkTheme())
	myWindow := myApp.NewWindow("Go Text Editor")
	myWindow.Resize(fyne.NewSize(800, 600))

	editor := widget.NewMultiLineEntry()
	var currentFilePath string

	// --- Terminal Output ---
	terminalOutput := widget.NewMultiLineEntry()
	terminalOutput.Disable()

	// --- Command Input ---
	commandInput := widget.NewEntry()
	commandInput.SetPlaceHolder("Enter command here...")

	// --- Run Command ---
	runCommand := func() {
		cmd := strings.TrimSpace(commandInput.Text)
		if cmd == "" {
			return
		}

		// Clear previous output or append
		output := terminalOutput.Text + "\n$ " + cmd + "\n"

		// Execute command
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			return
		}

		command := exec.Command(parts[0], parts[1:]...)
		stdout, err := command.CombinedOutput()

		// Remove ANSI escape codes
		ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		cleanOutput := ansiRegex.ReplaceAllString(string(stdout), "")

		if err != nil {
			output += fmt.Sprintf("Error: %v\n", err)
		} else {
			output += cleanOutput
		}

		terminalOutput.SetText(output)
		commandInput.SetText("")

		// Scroll to bottom
		terminalOutput.CursorRow = len(strings.Split(output, "\n"))
	}

	// --- Handle Enter key in command input ---
	commandInput.OnSubmitted = func(s string) {
		runCommand()
	}

	// --- Open File ---
	openFile := func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, myWindow)
				return
			}
			if reader == nil {
				return
			}
			defer reader.Close()

			data, readErr := io.ReadAll(reader)
			if readErr != nil {
				dialog.ShowError(fmt.Errorf("failed to read file: %w", readErr), myWindow)
				return
			}

			editor.SetText(string(data))
			currentFilePath = reader.URI().Path()
			myWindow.SetTitle("Go Text Editor - " + currentFilePath)
		}, myWindow)
	}

	// --- Save File ---
	saveFile := func() {
		if currentFilePath == "" {
			dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
				if err != nil {
					dialog.ShowError(err, myWindow)
					return
				}
				if uc == nil {
					return
				}
				defer uc.Close()

				currentFilePath = uc.URI().Path()
				myWindow.SetTitle("Go Text Editor - " + currentFilePath)

				_, writeErr := uc.Write([]byte(editor.Text))
				if writeErr != nil {
					dialog.ShowError(fmt.Errorf("failed to save file: %w", writeErr), myWindow)
					currentFilePath = ""
					myWindow.SetTitle("Go Text Editor - Dusk Rose")
				}
			}, myWindow)
		} else {
			err := os.WriteFile(currentFilePath, []byte(editor.Text), 0644)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to save file: %w", err), myWindow)
			}
		}
	}

	// --- Keyboard Shortcuts ---
	myWindow.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case "Command+O", "Control+O":
			openFile()
		case "Command+S", "Control+S":
			saveFile()
		}
	})

	// --- UI Layout ---
	openButton := widget.NewButton("Open", openFile)
	saveButton := widget.NewButton("Save", saveFile)
	buttonBar := container.NewHBox(openButton, saveButton)

	terminalContainer := container.NewBorder(
		commandInput,
		nil,
		nil,
		nil,
		terminalOutput,
	)

	splitContainer := container.NewVSplit(editor, terminalContainer)
	splitContainer.SetOffset(0.6)

	content := container.NewBorder(
		buttonBar,
		nil,
		nil,
		nil,
		splitContainer,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}