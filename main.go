package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

// previousNightHour set the max hour a picture can be in a previous album.
// e.g.: if you take a picture during a party at 3am, it will be put in the
// previous day album directory.
const previousNightHour = 6

func main() {
	err := run(os.Args[1:])
	if err != nil {
		panic(err)
	}
}

func run(args []string) error {
	root := &ffcli.Command{
		Exec: func(ctx context.Context, args []string) error {
			if len(args) < 2 {
				return errors.New("need a source and a destination")
			}
			src := args[0]
			dst := args[1]
			return scanAndOrg(ctx, src, dst)
		},
		UsageFunc: func(c *ffcli.Command) string {
			return `Usage: dirpic SRC DST
	SRC: the source directory to scan for images
	DST: the destination directory to write the images to in a chronological tree

For now, SRC and DST must be on the same mount, because it uses hard links to be efficient`
		},
	}
	exif.RegisterParsers(mknote.All...)
	return root.ParseAndRun(context.Background(), args)
}

// scanAndOrg will scan the src directory for data with EXIF metadata
// and hard link (or copy if not possible) into dst directory.
func scanAndOrg(ctx context.Context, src, dst string) error {
	return filepath.Walk(src, selectEXIFFile(src, dst))
}

func validExt(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg",
		".jpeg",
		".png",
		".gif",
		".tiff",
		".webp":
		return true
	default:
		return false
	}
}

func selectEXIFFile(src, dst string) func(path string, info fs.FileInfo, err error) error {
	return func(path string, info fs.FileInfo, err error) error {
		if path == "." || path == src {
			return nil
		}

		ext := filepath.Ext(path)
		if !validExt(ext) {
			log.Printf("%s : not correct extension", path)
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening %q: %w", path, err)
		}

		x, err := exif.Decode(f)
		if err != nil {
			log.Printf("decoding %q: %s", path, err)
		}

		// whatever picture taken before 6am will be added to the previous day instead
		// TODO: make it a config
		t := time.Date(0, time.January, 1, previousNightHour, 0, 0, 0, time.UTC)

		if x != nil {
			t, err = x.DateTime()
			if err != nil {
				log.Printf("%s: no date info", path)
				return nil
			}
		}

		if t.Hour() < 6 {
			t = t.Add(-6 * time.Hour)
		}

		dstSubDir := t.Format("2006/01/2006-01-02_")

		// Make it compatible with non UNIX OSes
		dstSubDir = filepath.FromSlash(dstSubDir)

		finalDst := filepath.Join(dst, dstSubDir)

		err = os.MkdirAll(finalDst, os.ModePerm)
		if err != nil {
			return fmt.Errorf("making dest dir '%s': %w", finalDst, err)
		}

		// TODO check for same filesystem
		// TODO start a copy if not on the same FS
		err = os.Link(path, filepath.Join(finalDst, info.Name()))
		if err != nil {
			if le, ok := err.(*os.LinkError); ok {
				log.Printf("linkerror: %q: %T: %s", path, le.Err, le.Err)
				return nil
			}
			if errors.Is(err, os.ErrExist) {
				log.Println("errexist:", err)
				return nil
			}
			return fmt.Errorf("link '%s/%s': %w", finalDst, info.Name(), err)
		}
		return nil
	}
}
