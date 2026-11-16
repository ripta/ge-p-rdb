# ge-p-rdb

A Go library for reading GE MR raw data files ("P-files", such as
`P12345.7`). It decodes the RDB header, the exam, series and image headers,
the data acquisition table and the raw k-space samples.

It needs no GE headers or SDK. It supports rdbm_rev 26.x and was checked
against two 26.002 files: a GRE and an FSE scan from an 8-channel coil.

## Usage

```go
import "github.com/ripta/ge-p-rdb/pkg/pfile"

f, err := pfile.Open("P20480.7")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

fmt.Println(f.Header.Series.Description, f.Header.Image.PSDName)

// Receivers, slices, echoes, views and points.
d := f.Dims()

// Map spatial slice 0 to its acquisition-order index in the data.
acq, err := f.AcqSlice(0)
if err != nil {
	log.Fatal(err)
}

// Receiver 0, echo 0. The baseline view is dropped.
blk, err := f.ReadBlock(0, acq, 0)
if err != nil {
	log.Fatal(err)
}
center := blk.At(d.Views/2, d.Points/2)
```

`pfile.New` takes any `io.ReaderAt` and a size, for data that is not in a
file. A header that does not make sense fails `Open` and `New`. A data
problem, such as a truncated file, does not. It is reported by `DataErr`
and by the methods that read samples.

## Format notes

- Everything is little-endian. The file starts with rdbm_rev as a float32.
- The RDB header holds byte offsets to each section. Exam, series and image
  fields are read relative to those offsets.
- Samples are stored as `[receiver][slice][echo][view][point]{re, im}`.
- Each block has nframes+1 views. View 0 is a baseline view of zeros.
- Components are int16 when point_size is 2, or int32 when it is 4.
- Slices are in acquisition order. Data acquisition table entry `i`
  describes spatial slice `i`. Its 1-based `slice_in_pass` gives the data
  index.

The field offsets come from the spec2nii/VESPA tables. Reconstructing the
sample files confirmed three things: receiver-major order, the baseline
view and the slice order.

## Limitations

- Only rdbm_rev 26.x is accepted. Only 26.002 has been tested.
- Only single-pass data can be read. Multi-pass files still decode their
  header.
- Both samples have one echo, so the position of the echo index is assumed.
- The tool, prescan, gradient and CTT sections are not decoded. Neither are
  bytes 40-71 of each acquisition table entry.

## pfinfo

`pfinfo` prints a summary of a P-file.

```
make build
build/pfinfo P20480.7
build/pfinfo -json P20480.7
build/pfinfo -png out/ P20480.7
```

- `-json` prints the full decoded header. That includes patient name, ID and
  date of birth.
- `-png` writes a rough sum-of-squares magnitude image per slice. It does no
  phase correction or view reordering. Some sequences show a half-FOV shift
  or FSE ghosting.

## Development

```
make            # list targets
make ci         # format check, vet, build and unit tests
make test-samples SAMPLES=~/Downloads
```

The unit tests use synthetic files. `test-samples` also runs against
`P20480_GRE.7` and `P21504_FSE.7` in `SAMPLES`. Those files are not in the
repo.

## License

MIT. See [LICENSE](LICENSE).
