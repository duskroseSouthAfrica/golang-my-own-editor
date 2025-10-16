package main

import (
    "fmt"
    "image/color"
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

// --- Custom Green Theme Structure (CORRECTED) ---
type customTheme struct {
    defaultTheme fyne.Theme 
}

// Ensure the theme is initialized correctly
func newCustomTheme() fyne.Theme {
    return &customTheme{
        defaultTheme: theme.DarkTheme(), // Call the function here
    }
}

func (t *customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
    if name == theme.ColorNameForeground {
        // Set the foreground color (used for most text) to green
        return color.RGBA{R: 0, G: 255, B: 0, A: 255} // Bright Green
    }
    // Delegate to the default DarkTheme color for all other colors
    return t.defaultTheme.Color(name, variant)
}

func (t *customTheme) Font(style fyne.TextStyle) fyne.Resource {
    return t.defaultTheme.Font(style)
}

func (t *customTheme) Size(name fyne.ThemeSizeName) float32 {
    return t.defaultTheme.Size(name)
}

func (t *customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
    return t.defaultTheme.Icon(name)
}

// --- END Custom Green Theme Structure ---

func main() {
    // FIX: Fyne requires a unique ID for preferences storage.
    // This resolves the error: "Preferences API requires a unique ID, use app.NewWithID()".
    myApp := app.NewWithID("com.example.gotexteditor") 
    myApp.Settings().SetTheme(newCustomTheme()) 
    
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

    // Helper to append output to the terminal - NOTE: This function must be called on the UI thread!
    appendToTerminal := func(s string) {
        output := terminalOutput.Text + "\n" + s
        terminalOutput.SetText(output)
        terminalOutput.CursorRow = len(strings.Split(output, "\n"))
        // Manually refresh the widget to ensure the UI updates immediately
        terminalOutput.Refresh() 
    }

    // --- Save File ---
    saveFile := func() error {
        if currentFilePath == "" {
            var saveErr error
            
            // Blocking file save dialog to ensure the file path is set before returning
            // Note: This relies on the synchronous nature or the context of the dialog completion.
            // For simplicity in a single file, we'll use a placeholder error handling.
            
            dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
                if err != nil {
                    saveErr = err
                    dialog.ShowError(err, myWindow)
                    return
                }
                if uc == nil {
                    saveErr = fmt.Errorf("save cancelled")
                    return
                }
                defer uc.Close()

                currentFilePath = uc.URI().Path()
                myWindow.SetTitle("Go Text Editor - " + currentFilePath)

                _, writeErr := uc.Write([]byte(editor.Text))
                if writeErr != nil {
                    saveErr = fmt.Errorf("failed to save file: %w", writeErr)
                    dialog.ShowError(saveErr, myWindow)
                    currentFilePath = ""
                    myWindow.SetTitle("Go Text Editor")
                }
            }, myWindow)
            
            // Since dialog.ShowFileSave is async, we can't reliably return an error here
            // We rely on the caller checking if the file path is set after the dialog runs.
            if currentFilePath == "" {
                 return fmt.Errorf("file not saved or save cancelled")
            }
            return nil
            
        } else {
            err := os.WriteFile(currentFilePath, []byte(editor.Text), 0644)
            if err != nil {
                dialog.ShowError(fmt.Errorf("failed to save file: %w", err), myWindow)
            }
            return err
        }
    }

    // --- Open File (Refreshes editor content) ---
    openFileContent := func() error {
        if currentFilePath == "" {
            return fmt.Errorf("no file path set")
        }
        data, err := os.ReadFile(currentFilePath)
        if err != nil {
            return fmt.Errorf("failed to read file content: %w", err)
        }
        editor.SetText(string(data))
        return nil
    }

    // --- Format File (New Function) ---
    formatFile := func() {
        if currentFilePath == "" {
            dialog.ShowInformation("Cannot Format", "Please save your file first before formatting.", myWindow)
            return
        }
        
        // 1. Save the current editor content to disk
        if err := saveFile(); err != nil {
            fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
                appendToTerminal(fmt.Sprintf("Error: Could not save file before formatting: %v", err))
            })
            return
        }

        // 2. Execute gofmt on the saved file using its full path
        fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
            appendToTerminal(fmt.Sprintf("$ gofmt -w %s", currentFilePath))
        })
        
        command := exec.Command("gofmt", "-w", currentFilePath)
        stdout, err := command.CombinedOutput()
        
        outputStr := string(stdout)
        
        // Wrap all UI updates in fyne.Do
        fyne.Do(func() {
            if err != nil {
                appendToTerminal(fmt.Sprintf("gofmt failed: %v\n%s", err, outputStr))
                return
            }

            // 3. Re-open the formatted content to update the editor
            if err := openFileContent(); err != nil {
                appendToTerminal(fmt.Sprintf("gofmt succeeded but failed to reload file: %v", err))
                return
            }
            
            appendToTerminal("gofmt successful: file formatted.")
        })
    }

    // --- Run Command (Now Asynchronous) ---
    runCommand := func() {
        cmd := strings.TrimSpace(commandInput.Text)
        if cmd == "" {
            return
        }

        // Disable input while command is running to prevent concurrency issues and show a loading state
        commandInput.Disable()

        // Execute command
        parts := strings.Fields(cmd)
        if len(parts) == 0 {
            commandInput.Enable() // Re-enable if command is empty
            return
        }
        
        // --- Smart Path Resolution for 'go run filename' ---
        // If the command is 'go run' followed by a filename, check if that filename 
        // matches the currently open file's base name, and if so, use the full path.
        isGoRunFile := parts[0] == "go" && len(parts) == 3 && parts[1] == "run"
        
        if isGoRunFile {
            filenameInCommand := parts[2]
            
            if currentFilePath != "" {
                // Extract the filename from the path using the OS path separator
                pathParts := strings.Split(currentFilePath, string(os.PathSeparator))
                currentFilename := pathParts[len(pathParts)-1]
                
                // If the command filename matches the open filename, substitute with the full path
                if filenameInCommand == currentFilename {
                    parts[2] = currentFilePath
                    fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
                        appendToTerminal(fmt.Sprintf("Note: Using absolute path for file: %s", currentFilePath))
                    })
                }
            }
        }
        // --- END Smart Path Resolution ---

        // --- Pre-execution logic (Saving and self-run check) ---
        if parts[0] == "go" {
            if len(parts) > 1 && (parts[1] == "run" || parts[1] == "build") {
                // 1. Save the file before running the code
                if err := saveFile(); err != nil {
                    if err.Error() != "file not saved or save cancelled" {
                         fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
                            appendToTerminal(fmt.Sprintf("Warning: Failed to save file before running command: %v", err))
                         })
                    } else {
                        fyne.Do(func() { // Wrap all UI calls
                            appendToTerminal("Command aborted because file save failed or was cancelled.")
                            commandInput.SetText("")
                            commandInput.Enable() // Crucial: Re-enable here on sync fail
                        })
                        return
                    }
                } else {
                    fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
                        appendToTerminal("File saved before running.")
                    })
                }

                // 2. Check if the user is trying to run the editor itself
                if parts[1] == "run" && (len(parts) == 2 && parts[2] == ".") {
                    if strings.HasSuffix(currentFilePath, "main.go") { // Simple check for the editor's main file
                        fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
                            appendToTerminal("Warning: Running the editor application from within itself often causes crashes.")
                            appendToTerminal("Please use 'go run' for other, simple programs.")
                        })
                    }
                }
            }
        }
        // --- END Pre-execution logic ---

        // Clear previous output or append
        fyne.Do(func() { // Wrap appendToTerminal since it modifies UI
            appendToTerminal("$ " + cmd)
        })
        
        // Handle the specific gofmt case (for completeness, though the button is preferred)
        if parts[0] == "gofmt" && len(parts) == 3 && parts[1] == "-w" {
            if parts[2] == currentFilePath || parts[2] == strings.Split(currentFilePath, "/")[len(strings.Split(currentFilePath, "/"))-1] {
                // Since this runs synchronously, we call the sync function directly
                formatFile() 
                fyne.Do(func() { // Wrap all UI calls
                    commandInput.SetText("")
                    commandInput.Enable() // Re-enable here on sync success
                })
                return
            }
        }

        // --- Execute Command in Goroutine (Async) ---
        go func() {
            command := exec.Command(parts[0], parts[1:]...)
            // IMPORTANT: Setting the working directory is often necessary for `go run .` to work correctly.
            // This assumes the user is running the editor from the project root.
            command.Dir = "." 
            
            stdout, err := command.CombinedOutput()
            
            // Remove ANSI escape codes (safe to do outside fyne.Do)
            ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
            cleanOutput := ansiRegex.ReplaceAllString(string(stdout), "")
            
            // FIX: Wrap all remaining UI updates in fyne.Do
            fyne.Do(func() {
                if err != nil {
                    appendToTerminal(fmt.Sprintf("Command Error: %v\n%s", err, cleanOutput))
                } else {
                    appendToTerminal(cleanOutput)
                }

                commandInput.SetText("")
                commandInput.Enable() // FIX: This is now safely executed on the UI thread
            })
        }() 
        // --- End Goroutine Execution ---
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
    
    // --- Handle Enter key in command input ---
    commandInput.OnSubmitted = func(s string) {
        runCommand()
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
    saveButton := widget.NewButton("Save", func() {
        saveFile()
        appendToTerminal("File saved.")
    })
    formatButton := widget.NewButton("Format (gofmt)", formatFile) // New Button
    
    buttonBar := container.NewHBox(openButton, saveButton, formatButton)

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
