package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/version"
)

// publishedRepo is where the release workflow pushes the stock sandbox: one
// multi-arch tag per nik release, public so a self-hosting household can pull
// it without credentials. A family running the Dockerfile nik ships gets that
// image instead of spending ten minutes building a byte-identical copy of it.
//
// The tag is the nik version rather than a floating one, so the sandbox a
// family runs matches the nik that was tested against it, and a rollback rolls
// both. TestPublishedRepoMatchesWorkflow keeps this in step with the workflow
// that fills it — drift here would have nik pulling an image nobody publishes.
//
// A var rather than a const only so the docker-backed test can point it at a
// registry on localhost; nothing at runtime writes it.
var publishedRepo = "ghcr.io/kilianc/nik-shell"

// stockMarker records the digest of the Dockerfile nik last seeded, next to
// the Dockerfile itself.
const stockMarker = ".Dockerfile.stock"

// publishedImage is the stock sandbox for this nik, or empty for a build that
// was never released: there is nothing to pull for a tag nobody published, and
// a build cut by hand is one whose Dockerfile is likely being worked on.
func publishedImage() string {
	if version.Number == "dev" {
		return ""
	}
	return publishedRepo + ":" + version.Number
}

// localImage is what a locally built sandbox is tagged. Versioned like the
// published one, because the alternative — a single floating tag — leaves an
// upgraded nik pointing at last release's image under a name that did not
// change, with nothing to notice it by.
func (s *Service) localImage() string {
	return s.dockerImage() + ":" + version.Number
}

// desiredImage is the sandbox this nik wants to be running. The stock
// Dockerfile on a released build is the one case nik can answer with a pull;
// a family who edited theirs, or a build with no published counterpart, gets
// the image built here from the file on disk.
func (s *Service) desiredImage(stock bool) string {
	if stock {
		if img := publishedImage(); img != "" {
			return img
		}
	}
	return s.localImage()
}

// ensureImage makes the sandbox image present and returns the one to run,
// which is not always the one asked for. The published image is a convenience
// and never a dependency: no network, a registry having a bad day, or an
// air-gapped host all fall back to building the same Dockerfile locally. That
// fallback is also what keeps the build path exercised rather than rotting
// into something nobody notices is broken until they need it.
func (s *Service) ensureImage(image string) (string, string, error) {
	exists, err := s.imageExists(image)
	if err != nil {
		return image, "", fmt.Errorf("check image: %w", err)
	}

	if exists {
		return image, "", nil
	}

	if !strings.HasPrefix(image, publishedRepo+":") {
		log, err := s.buildImage(image)
		return image, log, err
	}

	pullLog, err := s.pullImage(image)
	if err == nil {
		return image, pullLog, nil
	}

	slog.Warn("pull sandbox image, building it instead",
		"pkg", "shell", "image", image, "error", err)

	local := s.localImage()

	buildLog, err := s.buildImage(local)
	return local, pullLog + buildLog, err
}

func (s *Service) pullImage(image string) (string, error) {
	out, err := exec.Command("docker", "pull", image).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker pull %s: %s: %w",
			image, strings.TrimSpace(string(out)), err)
	}

	slog.Info("shell image pulled", "pkg", "shell", "image", image)
	return string(out), nil
}

// pruneImages drops the sandboxes this nik no longer runs. Each one is well
// over a gigabyte, and every release resolves a new tag, so without this a
// household that upgrades a few times fills a small host's disk with images
// nothing will ever start again. Best effort throughout: an image something
// still holds refuses to be removed, which is the answer we want.
func (s *Service) pruneImages(keep string) {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		return
	}

	for _, ref := range strings.Fields(string(out)) {
		if ref == keep || ref == s.localImage() {
			continue
		}

		sep := strings.LastIndex(ref, ":")
		if sep < 0 {
			continue
		}

		repo := ref[:sep]
		if repo != publishedRepo && repo != s.dockerImage() {
			continue
		}

		err = exec.Command("docker", "image", "rm", ref).Run()
		if err != nil {
			continue
		}

		slog.Info("removed superseded sandbox image", "pkg", "shell", "image", ref)
	}
}

func (s *Service) stockMarkerPath() string {
	return filepath.Join(filepath.Dir(s.dockerfilePath()), stockMarker)
}

// writeDefaultDockerfile lays down the Dockerfile nik ships and records its
// digest, which is what lets the next nik tell an untouched file from an
// edited one without having to guess.
func (s *Service) writeDefaultDockerfile() error {
	err := os.MkdirAll(filepath.Dir(s.dockerfilePath()), 0o755)
	if err != nil {
		return fmt.Errorf("create dockerfile dir: %w", err)
	}

	err = os.WriteFile(s.dockerfilePath(), []byte(defaultDockerfile), 0o644)
	if err != nil {
		return fmt.Errorf("write default dockerfile: %w", err)
	}

	s.markStock()
	return nil
}

func (s *Service) markStock() {
	want := digest([]byte(defaultDockerfile))

	have, err := os.ReadFile(s.stockMarkerPath())
	if err == nil && strings.TrimSpace(string(have)) == want {
		return
	}

	err = os.WriteFile(s.stockMarkerPath(), []byte(want+"\n"), 0o644)
	if err != nil {
		slog.Warn("record stock dockerfile digest", "pkg", "shell", "error", err)
	}
}

// matchesStockMarker reports whether the Dockerfile on disk is the one some
// nik seeded. The marker is what makes this exact across upgrades: the
// embedded default changes between releases, so comparing bytes against the
// current one alone would call every un-upgraded family's untouched file a
// customisation and build it locally forever after.
func (s *Service) matchesStockMarker(current []byte) bool {
	marked, err := os.ReadFile(s.stockMarkerPath())
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(marked)) == digest(current)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
