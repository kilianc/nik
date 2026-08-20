package apisvc

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
)

// Inspector answers read-only queries, through exactly the code the db_query
// brain tool uses — same read-only check, same redaction, same bounds. Two
// implementations would eventually have only one of those.
type Inspector struct {
	roConn *sql.DB
}

func NewInspector(roConn *sql.DB) *Inspector {
	return &Inspector{roConn: roConn}
}

func (i *Inspector) Query(ctx context.Context, query string) (any, error) {
	result, err := db.Query(ctx, i.roConn, query)
	if errors.Is(err, db.ErrNotReadOnly) {
		return nil, fmt.Errorf("%w: %s", api.ErrNotReadOnly, err)
	}
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Logs tails nikd's own log files. It is the first thing anybody wants from a
// nik that is up and not answering, and until now it needed a shell on the box.
type Logs struct {
	logPath string
	errPath string
}

func NewLogs(logPath, errPath string) *Logs {
	return &Logs{logPath: logPath, errPath: errPath}
}

func (l *Logs) Tail(ctx context.Context, errorsOnly bool, lines int) ([]string, error) {
	path := l.logPath
	if errorsOnly {
		path = l.errPath
	}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// A nik that has not written a log yet is not an error; it is a nik
		// that has not written a log yet.
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer file.Close()

	// A ring buffer rather than reading the file in: nik's log grows without
	// bound between prunes, and a cell has 320 MB.
	ring := make([]string, lines)
	count := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	for scanner.Scan() {
		ring[count%lines] = scanner.Text()
		count++
	}

	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}

	if count < lines {
		return ring[:count], nil
	}

	out := make([]string, 0, lines)
	for i := range lines {
		out = append(out, ring[(count+i)%lines])
	}

	return out, nil
}

// Restarter ends the process and lets the service manager bring it back.
//
// Same mechanism as the `restart` brain tool: SIGTERM, so in-flight work
// finishes rather than being cut off. A daemon nobody is managing does not
// come back, which is why nikctl says so rather than pretending.
type Restarter struct{}

func NewRestarter() *Restarter { return &Restarter{} }

func (r *Restarter) Restart(ctx context.Context) error {
	slog.Info("restart requested over the api", "pkg", "apisvc")

	// Async so the response goes out first. A restart whose caller never
	// learns it was accepted looks like a hang.
	go func() {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()

	return nil
}

// Shell runs a command in nik's sandbox.
type Shell struct {
	run func(ctx context.Context, command string, timeoutSeconds int) (string, bool, int, error)
}

// NewShell takes the runner rather than the service, so this package does not
// import internal/shell and drag docker and tmux into everything that touches
// the API.
func NewShell(run func(ctx context.Context, command string, timeoutSeconds int) (string, bool, int, error)) *Shell {
	return &Shell{run: run}
}

func (s *Shell) Run(ctx context.Context, command string, timeoutSeconds int) (api.ShellResult, error) {
	output, alive, code, err := s.run(ctx, command, timeoutSeconds)
	if err != nil {
		return api.ShellResult{}, err
	}

	return api.ShellResult{
		Output:   strings.TrimRight(output, "\n"),
		ExitCode: code,
		Running:  alive,
	}, nil
}
