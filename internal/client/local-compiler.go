package client

import (
	"bufio"
	"bytes"
	"math/rand"
	"os/exec"
	"path/filepath"
	"strings"
)

// LocalCompiler ...
type LocalCompiler struct {
	name                     string
	inFile                   string
	outFile                  string
	remoteCmdArgs            []string
	localCmdArgs             []string
	RemoteCompilationAllowed bool
}

func isSourceFile(file string) bool {
	return strings.HasSuffix(file, ".cpp") || strings.HasSuffix(file, ".cc") || strings.HasSuffix(file, ".cxx") || strings.HasSuffix(file, ".c")
}

// MakeLocalCompiler ...
func MakeLocalCompiler(compilerArgs []string) *LocalCompiler {
	compiler := LocalCompiler{}
	var remoteCompilationAllowed = true

	for i := 1; i < len(compilerArgs); i++ {
		arg := compilerArgs[i]
		if len(arg) == 0 {
			continue
		}
		if arg[0] == '-' {
			if arg == "-o" {
				if i+1 < len(compilerArgs) {
					compiler.outFile, _ = filepath.Abs(compilerArgs[i+1])
					i++
					continue
				} else {
					remoteCompilationAllowed = false
				}
			} else if strings.HasPrefix(arg, "-o") {
				compiler.outFile, _ = filepath.Abs(arg[2:])
				continue
			} else if strings.HasSuffix(arg, "=native") {
				remoteCompilationAllowed = false
			}
		} else if isSourceFile(arg) {
			if len(compiler.inFile) != 0 {
				remoteCompilationAllowed = false
			}
			compiler.inFile, _ = filepath.Abs(arg)
			continue
		}
		compiler.remoteCmdArgs = append(compiler.remoteCmdArgs, arg)
	}

	compiler.name = compilerArgs[0]
	if len(compilerArgs) > 1 {
		compiler.localCmdArgs = compilerArgs[1:]
	}
	compiler.RemoteCompilationAllowed = remoteCompilationAllowed && len(compiler.inFile) != 0 && strings.HasSuffix(compiler.outFile, ".o")
	return &compiler
}

// CollectHeaders ...
func (compiler *LocalCompiler) CollectHeaders() ([]string, error) {
	compilerProc := exec.Command(compiler.name, append(compiler.remoteCmdArgs, compiler.inFile, "-o", "/dev/stdout", "-M")...)
	var compilerStdout, compilerStderr bytes.Buffer
	compilerProc.Stdout = &compilerStdout
	compilerProc.Stderr = &compilerStderr
	if err := compilerProc.Run(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(compilerStdout.Bytes()))
	scanner.Split(bufio.ScanWords)
	headersMap := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#pragma GCC pch_preprocess") {
			continue
		}
		if line == "\\" || isSourceFile(line) || strings.HasSuffix(line, ".o") || strings.HasSuffix(line, ".o:") {
			continue
		}
		filePath, _ := filepath.Abs(line)
		headersMap[filePath] = true
	}

	headers := make([]string, 0, len(headersMap))
	for h := range headersMap {
		headers = append(headers, h)
	}

	rand.Shuffle(len(headers), func(i, j int) { headers[i], headers[j] = headers[j], headers[i] })
	return headers, nil
}

// CompileLocally ...
func (compiler *LocalCompiler) CompileLocally() (retCode int, stdout []byte, stderr []byte) {
	compilerProc := exec.Command(compiler.name, compiler.localCmdArgs...)
	var compilerStdout, compilerStderr bytes.Buffer
	compilerProc.Stdout = &compilerStdout
	compilerProc.Stderr = &compilerStderr
	_ = compilerProc.Run()

	retCode = compilerProc.ProcessState.ExitCode()
	stdout = compilerStdout.Bytes()
	stderr = compilerStderr.Bytes()
	return
}
