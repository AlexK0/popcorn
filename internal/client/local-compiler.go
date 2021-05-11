package client

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
)

// LocalCompiler ...
type LocalCompiler struct {
	name                     string
	inFile                   string
	outFile                  string
	remoteCmdArgs            []string
	dirsIquote               []string
	dirsI                    []string
	dirsIsystem              []string
	localCmdArgs             []string
	RemoteCompilationAllowed bool
}

func isSourceFile(file string) bool {
	return strings.HasSuffix(file, ".cpp") || strings.HasSuffix(file, ".cc") || strings.HasSuffix(file, ".cxx") || strings.HasSuffix(file, ".c")
}

func MakeLocalCompiler(compilerArgs []string) *LocalCompiler {
	compiler := LocalCompiler{}
	var remoteCompilationAllowed = true

	compiler.dirsIquote = make([]string, 0, 2)
	compiler.dirsI = make([]string, 0, 2)
	compiler.dirsIsystem = make([]string, 0, 2)

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
			} else if strings.HasSuffix(arg, "=native") || arg == "-I-" ||
				// TODO think about it
				strings.HasPrefix(arg, "-idirafter") || strings.HasPrefix(arg, "--sysroot") || strings.HasPrefix(arg, "-isysroot") {
				remoteCompilationAllowed = false
			} else if arg == "-I" {
				if i+1 < len(compilerArgs) {
					compiler.dirsI = append(compiler.dirsI, compilerArgs[i+1])
					i++
					continue
				} else {
					remoteCompilationAllowed = false
				}
			} else if strings.HasPrefix(arg, "-I") {
				compiler.dirsI = append(compiler.dirsI, arg[2:])
				continue
			} else if arg == "-iquote" {
				if i+1 < len(compilerArgs) {
					compiler.dirsIquote = append(compiler.dirsIquote, compilerArgs[i+1])
					i++
					continue
				} else {
					remoteCompilationAllowed = false
				}
			} else if strings.HasPrefix(arg, "-iquote") {
				compiler.dirsIquote = append(compiler.dirsIquote, arg[7:])
				continue
			} else if arg == "-isystem" {
				if i+1 < len(compilerArgs) {
					compiler.dirsIsystem = append(compiler.dirsIsystem, compilerArgs[i+1])
					i++
					continue
				} else {
					remoteCompilationAllowed = false
				}
			} else if strings.HasPrefix(arg, "-isystem") {
				compiler.dirsIsystem = append(compiler.dirsIsystem, arg[8:])
				continue
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

func extractHeaders(rawOut []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(rawOut))
	scanner.Split(bufio.ScanWords)
	headers := make([]string, 0, 16)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "#pragma" && scanner.Scan() && scanner.Text() == "GCC" && scanner.Scan() && scanner.Text() == "pch_preprocess" && scanner.Scan() {
			headers = append(headers, strings.Trim(scanner.Text(), "\""))
			continue
		}

		if line == "\\" || isSourceFile(line) || strings.HasSuffix(line, ".o") || strings.HasSuffix(line, ".o:") {
			continue
		}
		headers = append(headers, line)
	}
	return common.NormalizePaths(headers)
}

func (compiler *LocalCompiler) addIncludeDirsFrom(rawOut string) {
	const (
		dirsIquoteStart = "#include \"...\""
		dirsIStart      = "#include <...>"
		dirsEnd         = "End of search list"

		ProcessUnknown    = 0
		ProcessDirsIquote = 1
		ProcessDirsI      = 2
	)

	processType := ProcessUnknown
	for _, line := range strings.Split(string(rawOut), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, dirsIquoteStart) {
			processType = ProcessDirsIquote
		} else if strings.HasPrefix(line, dirsIStart) {
			processType = ProcessDirsI
		} else if strings.HasPrefix(line, dirsEnd) {
			return
		} else if strings.HasPrefix(line, "/") {
			switch processType {
			case ProcessDirsIquote:
				compiler.dirsIquote = append(compiler.dirsIquote, line)
			case ProcessDirsI:
				if strings.HasPrefix(line, "/usr/") {
					compiler.dirsIsystem = append(compiler.dirsIsystem, line)
				} else {
					compiler.dirsI = append(compiler.dirsI, line)
				}
			}
		}
	}
}

func (compiler *LocalCompiler) MakeRemoteCmd(extraArgs ...string) []string {
	compiler.dirsIquote = common.NormalizePaths(compiler.dirsIquote)
	compiler.dirsI = common.NormalizePaths(compiler.dirsI)
	compiler.dirsIsystem = common.NormalizePaths(compiler.dirsIsystem)

	cmd := make([]string, 0, 2*(len(compiler.dirsIquote)+len(compiler.dirsI)+len(compiler.dirsIsystem))+len(compiler.remoteCmdArgs)+len(extraArgs))
	for _, dir := range compiler.dirsIquote {
		cmd = append(cmd, "-iquote", dir)
	}
	for _, dir := range compiler.dirsI {
		cmd = append(cmd, "-I", dir)
	}
	for _, dir := range compiler.dirsIsystem {
		cmd = append(cmd, "-isystem", dir)
	}

	cmd = append(cmd, compiler.remoteCmdArgs...)
	return append(cmd, extraArgs...)
}

func (compiler *LocalCompiler) CollectFilesAndUpdateIncludeDirs() ([]string, error) {
	cmd := compiler.MakeRemoteCmd(compiler.inFile, "-o", "/dev/stdout", "-M", "-Wp,-v")
	compilerProc := exec.Command(compiler.name, cmd...)
	var compilerStdout, compilerStderr bytes.Buffer
	compilerProc.Stdout = &compilerStdout
	compilerProc.Stderr = &compilerStderr
	if err := compilerProc.Run(); err != nil {
		return nil, err
	}

	compiler.addIncludeDirsFrom(compilerStderr.String())
	return append(extractHeaders(compilerStdout.Bytes()), compiler.inFile), nil
}

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
