package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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
			err := scanAndOrg(ctx, src, dst)

			return err
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
	return filepath.WalkDir(src, selectEXIFFile(src, dst))
}

func validExt(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case
		".jpg",
		".jpeg",
		".heic",
		".heif",
		//".gif",
		".tiff",
		".tif",
		".avi",
		".mpg",
		".mp4",
		".mov":
		//".webp",
		//".png":
		return true
	default:
		return false
	}
}

func selectEXIFFile(src, dst string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, inerr error) (err error) {
		if inerr != nil {
			return err
		}

		if path == "." || path == src {
			return nil
		}

		// if d.IsDir() {
		//	name := d.Name()
		//	pref, dirNumStr, ok := strings.Cut(name, ".")

		//	if ok {
		//		dirNum, err := strconv.Atoi(dirNumStr)
		//		if err == nil {
		//			if pref == "recup_dir" && dirNum <= 7920 {
		//				return fs.SkipDir
		//			}
		//		}
		//	}
		//}

		ext := filepath.Ext(path)
		if !validExt(ext) {
			// log.Printf("%s : not correct extension", path)
			return nil
		}

		// info, err := d.Info()
		// if err != nil {
		// 	return fmt.Errorf("error reading file info: %w", err)
		// }
		//
		// if info.Size() <= 500_000_000 {
		// 	return nil
		// }

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening %q: %w", path, err)
		}

		fileNameDate, err := getDateFromFileName(f.Name())
		if err != nil {
			slog.Info("getting date from file name", "filename", f.Name(), "error", err)
		}

		x, err := exif.Decode(f)
		if err != nil {
			if ext != ".mp4" {
				slog.Info("exif: decoding", "path", path, "error", err)
			}
		}

		// whatever picture taken before 6am will be added to the previous day instead
		// TODO: make it a config
		t := time.Date(0, time.January, 1, previousNightHour, 0, 0, 0, time.UTC)

		if x != nil {
			t, err = x.DateTime()
			if err != nil {
				slog.Info("no date info", "path", path, "error", err)
			}
		}
		if x == nil {
			t = fileNameDate
		}

		if t.Hour() < 6 {
			t = t.Add(-6 * time.Hour)
		}

		dstSubDir := t.Format("2006/01/2006-01-02_")

		// Make it compatible with non UNIX OSes
		dstSubDir = filepath.FromSlash(dstSubDir)

		finalDst := filepath.Join(dst, dstSubDir)

		dst := filepath.Join(finalDst, d.Name())

		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("stat-ing %q: %w", path, err)
		}

		di, err := os.Stat(dst)
		if err == nil {
			// there is destination file, so we can optimize with last update checks
			// and hard link similarities

			if os.SameFile(fi, di) {
				// they have been hard linked already, we should just skip it
				return nil
			}

			if di.ModTime().Compare(fi.ModTime()) == 0 {
				// we can skip as they have not been modified since last time
				return nil
			}
		}
		err = os.MkdirAll(finalDst, os.ModePerm)
		if err != nil {
			return fmt.Errorf("making dest dir %q: %w", finalDst, err)
		}

		// TODO check for same filesystem
		// TODO start a copy if not on the same FS

		err = os.Link(path, dst)
		if err != nil {
			if le, ok := err.(*os.LinkError); ok {
				err = fileCopy(path, dst)
				if err != nil {
					slog.Info("file copy", "path", path, "error", err, "error_type", reflect.TypeOf(err))
					slog.Info("link", "path", path, "error", le.Err, "error_type", reflect.TypeOf(le.Err))
					return nil
				}
				return nil
			}
			if errors.Is(err, os.ErrExist) {
				slog.Info("errexist", "error", err)
				return nil
			}
			return fmt.Errorf("link '%s/%s': %w", finalDst, d.Name(), err)
		}
		return nil
	}
}

func fileCopy(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY, 0o660)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	return nil
}

func getDateFromFileName(filePath string) (time.Time, error) {
	_, fileName := filepath.Split(filePath)

	t, errG := getDateFromSamsungGalaxyFileName(fileName)
	if errG == nil {
		return t, nil
	}

	t, errS := getDateFromSignalFileName(fileName)
	if errS == nil {
		return t, nil
	}
	return t, errors.Join(errG, errS)
}

func getDateFromSamsungGalaxyFileName(fileName string) (time.Time, error) {
	// TODO: add file name date from signal exported files
	if len(fileName) < 16 {
		return time.Time{}, errors.New("wrong file name time format")
	}

	dateStr := fileName[:15] //strings.TrimSuffix(fileName, ext)

	t, err := time.Parse("20060102_150405", dateStr)
	if err != nil {
		return time.Time{}, err
	}

	return t, nil
}

func getDateFromSignalFileName(fileName string) (time.Time, error) {
	const signalFileNameTemplate = "signal-2006-01-02-15-04-05"
	if !strings.HasPrefix(fileName, "signal") ||
		len(fileName) < len(signalFileNameTemplate) {
		return time.Time{}, errors.New("not a signal media filename")
	}

	strippedFileName := fileName[:len(signalFileNameTemplate)]

	t, err := time.Parse(signalFileNameTemplate, strippedFileName)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
