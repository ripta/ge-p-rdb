// Package pfile reads GE MR raw data files ("P-files"), such as P12345.7.
//
// A P-file is a little-endian RDB header followed by raw k-space samples.
// The header begins with rdbm_rev as a float32. It then holds byte offsets
// to its sub-sections: the acquisition table, exam, series, image and
// others. This package reads fields relative to those offsets. It does not
// assume that the sections are contiguous.
//
// Only rdbm_rev 26.x is supported. The field offsets come from the
// spec2nii/VESPA tables. They were checked against two 26.002 files, an FSE
// and a GRE scan from an 8-channel coil.
//
// # Data layout
//
// For a single-pass scan, samples are stored as
//
//	[receiver][slice][echo][view][point]{re, im}
//
// Each (receiver, slice, echo) block holds nframes+1 views. View 0 is a
// baseline view and is normally all zeros. Each component is an int16 when
// point_size is 2, or an int32 when point_size is 4.
//
// Receiver-major order, the baseline view and the slice order were confirmed
// by reconstructing the sample files. Both samples have one echo, so the
// position of the echo index is assumed, not confirmed.
//
// The slice index in the data is the acquisition order, not the spatial
// order. Use [File.AcqSlice] to map between the two.
package pfile
