package branchmeta

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/pinpt/ripsrc/ripsrc/gittime"

	"github.com/pinpt/ripsrc/ripsrc/pkg/logger"
)

type Opts struct {
	Logger         logger.Logger
	RepoDir        string
	UseOrigin      bool
	IncludeDefault bool
}

type BranchWithCommitTime struct {
	Name                string
	Commit              string
	CommitCommitterTime time.Time
}

func Get(ctx context.Context, opts Opts) (res []BranchWithCommitTime, _ error) {
	defaultBranch, err := getDefaultBranch(opts)
	if err != nil {
		return nil, err
	}
	args := []string{
		"for-each-ref",
		"--format",
		"%(objectname)@@@%(refname:short)@@@%(committerdate)",
	}
	if opts.UseOrigin {
		args = append(args, "refs/remotes/origin")
	} else {
		args = append(args, "refs/heads")
	}
	data, err := execCommand("git", opts.RepoDir, args)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		line := string(line)
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if line[0] == '(' {
			// not a branch, but a entry for detached head
			// (HEAD detached at faeab7d)
			continue
		}
		parts := strings.SplitN(line, "@@@", 3)
		if len(parts) != 3 {
			panic("unexpected format")
		}
		b := BranchWithCommitTime{}
		b.Commit = parts[0]
		b.Name = parts[1]
		date, err := gittime.Parse(parts[2])
		if err != nil {
			panic("invalid date format")
		}
		b.CommitCommitterTime = date
		if opts.UseOrigin {
			if !strings.HasPrefix(b.Name, "origin/") {
				panic("branch name does not have origin/ prefix")
			}
			b.Name = strings.TrimPrefix(b.Name, "origin/")
		}
		if b.Name == "HEAD" {
			continue
		}
		if !opts.IncludeDefault && b.Name == defaultBranch {
			continue
		}
		res = append(res, b)
	}
	sort.Slice(res, func(i, j int) bool {
		a := res[i]
		b := res[j]
		return a.Name < b.Name
	})
	return
}

func getDefaultBranch(opts Opts) (string, error) {
	args := []string{
		"symbolic-ref",
		"--short",
		"HEAD",
	}
	data, err := execCommand("git", opts.RepoDir, args)
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(string(data))
	if len(res) == 0 {
		return "", errors.New("could not get the default branch name")
	}
	return res, nil
}

func execCommand(command string, dir string, args []string) ([]byte, error) {
	out := bytes.NewBuffer(nil)
	c := exec.Command(command, args...)
	c.Dir = dir
	c.Stdout = out
	err := c.Run()
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
