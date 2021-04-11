# Dir Pic

A simple tool to organize pictures 🖼️ that have date EXIF metadata into date 📆 folders.

```
- 2021
  - 01
    - 2021-12-01_
      - pic1.jpg
      - pic2.jpg
- 2020
  - 12
    - 2020-12-31_
      - pic9998.jpg
      - pic9999.jpg
```

## Usage

```
Usage: dirpic SRC DST
	SRC: the source directory to scan for images
	DST: the destination directory to write the images to in a chronological tree

For now, SRC and DST must be on the same mount, because it uses hard links to be efficient
```
