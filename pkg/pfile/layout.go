package pfile

// Field offsets for rdbm_rev 26.x.
//
// The offsets were derived from the rdbm_rev 26-31 field list in spec2nii
// (spec2nii/GE/ge_hdr_fields.py). spec2nii took it from the VESPA project
// (Duke University, BSD license). That list is cumulative from the start of
// the file. Here the exam, series and image offsets are relative to their
// section pointers (off_exam, off_series, off_image). In 26.002 files those
// sections start at 193660, 195620 and 198180.
//
// Character fields are fixed-size and NUL-padded. Their sizes are given at
// the parse site.

// RDB header offsets, from the start of the file.
const (
	offRev            = 0
	offOffData        = 4
	offOffPerPass     = 8
	offOffUnlockRaw   = 12
	offOffDataAcqTab  = 16
	offOffNexTab      = 20
	offOffNexAbortTab = 24
	offOffTool        = 28
	offOffExam        = 32
	offOffSeries      = 36
	offOffImage       = 40
	offOffPS          = 44
	offOffGradData    = 48
	offOffCTTData     = 52

	offScanDate        = 92
	offScanTime        = 102
	offLogo            = 110
	offFileContents    = 120
	offDataCollectType = 132
	offNPasses         = 140
	offNSlices         = 144
	offNEchoes         = 146
	offNAvs            = 148
	offNFrames         = 150
	offFrameSize       = 156
	offPointSize       = 158
	offDAXRes          = 178
	offDAYRes          = 180

	// Four {start_rcv, stop_rcv} int16 pairs.
	offDAB = 264

	// rhuser0..rhuser19 are contiguous here. rhuser20..rhuser48 start at
	// offRHUser20.
	offRHUser0 = 280

	offSpectralWidth = 432
	offCSIDims       = 436
	offXCSI          = 438
	offYCSI          = 440
	offZCSI          = 442
	offROILenX       = 444
	offROILenY       = 448
	offROILenZ       = 452
	offPSMPSFreq     = 488
	offUserUsageTag  = 924
	offRHUser20      = 936
	offRawPassSize   = 1540

	rdbHeaderMinSize = offRawPassSize + 8
)

// Exam offsets, relative to off_exam.
const (
	exMagStrength = 572
	exDateTime    = 580
	exNo          = 696
	exPatSex      = 720
	exRefPhy      = 1057
	exSysID       = 1320
	exHospName    = 1351
	exVersCre     = 1388
	exUniqSysID   = 1392
	exStudyUID    = 1428
	exPatName     = 1524
	exPatID       = 1589
	exReqNum      = 1654
	exDateOfBirth = 1671

	examMinSize = exDateOfBirth + 9
)

// Series offsets, relative to off_series.
const (
	sePosition    = 640
	seEntry       = 644
	seNo          = 736
	seDesc        = 982
	seAnRef       = 1065
	seSeriesUID   = 1145
	seLandmarkUID = 1177

	seriesMinSize = seLandmarkUID + 32
)

// Image offsets, relative to off_image.
const (
	imDFOV        = 304
	imScanSpacing = 320
	imLoc         = 324
	imNEX         = 332

	// The image user floats come in three runs: user0..user22,
	// user23..user24 and user25..user48.
	imUser0  = 356
	imUser23 = 456
	imUser25 = 524

	// Each of these is an R, A, S float32 triple.
	imCtr  = 696
	imTLHC = 720
	imTRHC = 732
	imBRHC = 744

	imTR        = 1056
	imTI        = 1060
	imTE        = 1064
	imRawRunNum = 1108
	imNumEcho   = 1378
	imFlip      = 1412
	imCTyp      = 1434
	imFreqDir   = 1500
	imPSDName   = 1632
	imProjName  = 1665
	imPSDIName  = 1678
	imCName     = 1749
	imImageUID  = 1817

	imageMinSize = imImageUID + 32
)

// Data acquisition table entry layout. The entry size was measured from
// 26.002 files. Bytes 40-71 hold fields that are not decoded here.
const (
	acqEntrySize   = 72
	acqPassNumber  = 0
	acqSliceInPass = 2
	acqGWPoint1    = 4
	acqGWPoint2    = 16
	acqGWPoint3    = 28
)
