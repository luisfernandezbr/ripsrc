package ripsrc

import (
	"context"
	"os"
	"time"

	"github.com/pinpt/ripsrc/ripsrc/parentsgraph"

	"github.com/pinpt/ripsrc/ripsrc/commitmeta"
	"github.com/pinpt/ripsrc/ripsrc/fileinfo"
	"github.com/pinpt/ripsrc/ripsrc/gitexec"
	"github.com/pinpt/ripsrc/ripsrc/pkg/logger"

	"github.com/pinpt/ripsrc/ripsrc/history3/process"
)

// Opts is configuration for running ripsrc on a single repo.
type Opts struct {
	// RepoDir git repo to run commands on.
	RepoDir string

	// Logger object for info and debug.
	Logger logger.Logger

	// CheckpointsDir is the directory to store incremental data cache for this repo.
	// If empty, directory is created inside repoDir.
	CheckpointsDir string

	// NoStrictResume forces incremental processing to avoid checking that it continues from the same commit in previously finished on. Since incrementals save a large number of previous commits, it works even starting on another commit.
	NoStrictResume bool

	// CommitFromIncl process starting from this commit (including this commit).
	CommitFromIncl string

	// CommitFromMakeNonIncl by default we start from passed commit and include it. Set CommitFromMakeNonIncl to true to avoid returning it, and skipping reading/writing checkpoint.
	CommitFromMakeNonIncl bool

	// IncrementalIgnoreBranchesOlderThan provides a way to ignore old branches in incremental processing.
	// Default is time.Now() - 90 * day
	// BUG: this field is ignored, only processing HEAD branch in incrementals right now
	IncrementalIgnoreBranchesOlderThan time.Time

	// AllBranches set to true to process all branches. If false, processes HEAD only.
	// BUG: in incrementals only processing HEAD branch
	AllBranches bool

	// BranchesUseOrigin by default ripsrc lists only local branches when using Branches method. Set this to true to use origin/ branches instead.
	BranchesUseOrigin bool

	// PullRequestSHAs is a list of custom sha references to process similar to branches returned from the repo.
	PullRequestSHAs []string
}

// Ripsrc runs on a single repo.
type Ripsrc struct {
	GitProcessTimings process.Timing
	CodeInfoTimings   *CodeInfoTimings

	opts            Opts
	gitExecPrepared bool

	commitMeta map[string]commitmeta.Commit

	fileInfo *fileinfo.Process

	commitGraph *parentsgraph.Graph
}

func New(opts Opts) *Ripsrc {

	if opts.Logger == nil {
		opts.Logger = logger.NewDefaultLogger(os.Stdout)
	}

	s := &Ripsrc{}
	s.opts = opts
	s.CodeInfoTimings = &CodeInfoTimings{}
	s.fileInfo = fileinfo.New()
	return s
}

var gitCommand = "git"

func (s *Ripsrc) prepareGitExec(ctx context.Context) error {
	if s.gitExecPrepared {
		return nil
	}
	return gitexec.Prepare(ctx, gitCommand, s.opts.RepoDir)
}

func (s *Ripsrc) buildCommitGraph(ctx context.Context) error {
	if s.commitGraph != nil {
		return nil
	}

	s.commitGraph = parentsgraph.New(parentsgraph.Opts{
		RepoDir:     s.opts.RepoDir,
		AllBranches: s.opts.AllBranches,
		Logger:      s.opts.Logger,
	})

	return s.commitGraph.Read()
}
