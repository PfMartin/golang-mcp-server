package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	mcpServer := server.NewMCPServer("python-executor", "1.0.0")

	pythonTool := mcp.NewTool(
		"execute-python",
		mcp.WithDescription("Execute Python code in isolated environment. Playwright and headless browser are available for web scraping. Use this tool when you need real time information. Only output printed to stdout or stderr is returned so ALWAYS use print statements! Please note all code is run in an ephemeral container so modules and code do NOT persist!"),
		mcp.WithString("code", mcp.Description("The Python code to execute"), mcp.Required()),
		mcp.WithString("modules", mcp.Description("Comma-separated list of Python modules your code requires. If your code requires external modules you MUST pass them here! These will installed automatically.")),
	)

	mcpServer.AddTool(pythonTool, handlePythonExecution)

	// Run as stdio server
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// handlePythonExecution handles the execute-python tool calls
func handlePythonExecution(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	code, ok := request.Params.Arguments.(map[string]any)["code"].(string)
	if !ok {
		return mcp.NewToolResultError("Missing or invalid code argument"), nil
	}

	// Handle optional modules argument
	var modules []string
	if modulesStr, ok := request.Params.Arguments.(map[string]any)["modules"].(string); ok &&
		modulesStr != "" {
		modules = strings.Split(modulesStr, ",")
	}

	tmpDir, err := os.MkdirTemp("", "python_repl")
	if err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("Failed to create temp dir: %v", err),
		), nil
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(path.Join(tmpDir, "script.py"), []byte(code), 0644)
	if err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("Failed to write script to file: %v", err),
		), nil
	}

	dockerArgs := []string{
		"run",
		"--rm",
		"-v",
		fmt.Sprintf("%s:/app", tmpDir),
		"mcr.microsoft.com/playwright/python:v1.49.1-noble",
	}
	shArgs := []string{}

	if len(modules) > 0 {
		shArgs = append(shArgs, "python", "-m", "pip", "install", "--quiet")
		shArgs = append(shArgs, modules...)
		shArgs = append(shArgs, "&&")
	}

	shArgs = append(shArgs, "python", path.Join("app", "script.py"))
	dockerArgs = append(dockerArgs, "sh", "-c", strings.Join(shArgs, " "))

	cmd := exec.Command("docker", dockerArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return mcp.NewToolResultError(
				fmt.Sprintf(
					"Python exited with code %d: %s",
					exitError.ExitCode(),
					string(exitError.Stderr),
				),
			), nil
		}
		return mcp.NewToolResultError(
			fmt.Sprintf("Execution failed: %v", err),
		), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}
