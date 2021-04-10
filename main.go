package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

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
	}
	exif.RegisterParsers(mknote.All...)
	return root.ParseAndRun(context.Background(), args)
}

// scanAndOrg will scan the src directory for data with EXIF metadata
// and hard link (or copy if not possible) into dst directory.
func scanAndOrg(ctx context.Context, src, dst string) error {
	return filepath.Walk(src, selectEXIFFile(dst))
}

func validExt(ext string) bool {
	oks := map[string]struct{}{
		".jpg":  {},
		".jpeg": {},
		".png":  {},
		".gif":  {},
		".tiff": {},
		".webp": {},
	}
	_, ok := oks[ext]
	return ok
}

func selectEXIFFile(dst string) func(path string, info fs.FileInfo, err error) error {
	return func(path string, info fs.FileInfo, err error) error {
		if path == "." {
			return nil
		}

		ext := filepath.Ext(path)
		ext = strings.ToLower(ext)
		if !validExt(ext) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}

		x, err := exif.Decode(f)
		if err != nil {
			return err
		}

		t, err := x.DateTime()
		if err != nil {
			log.Printf("%s: no date info", path)
			return nil
		}

		dstSubDir := t.Format("2006/01/2006-01-02_")
		finalDst := filepath.Join(dst, dstSubDir)

		err = os.MkdirAll(finalDst, os.ModePerm)
		if err != nil {
			return err
		}

		err = os.Link(path, filepath.Join(finalDst, info.Name()))
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				log.Println(err)
				return nil
			}
			return err
		}
		return nil
	}
}
