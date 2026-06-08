package camera

import "strconv"

type CameraConfig struct {
	// Boolean flags
	VFlip              bool `yaml:"vflip,omitempty" json:"vflip,omitempty"`
	HFlip              bool `yaml:"hflip,omitempty" json:"hflip,omitempty"`
	NoPreview          bool `yaml:"no_preview,omitempty" json:"no_preview,omitempty"`
	Fullscreen         bool `yaml:"fullscreen,omitempty" json:"fullscreen,omitempty"`
	QtPreview          bool `yaml:"qt_preview,omitempty" json:"qt_preview,omitempty"`
	Flush              bool `yaml:"flush,omitempty" json:"flush,omitempty"`
	LoresPAR           bool `yaml:"lores_par,omitempty" json:"lores_par,omitempty"`
	NoRaw              bool `yaml:"no_raw,omitempty" json:"no_raw,omitempty"`
	Datetime           bool `yaml:"datetime,omitempty" json:"datetime,omitempty"`
	Timestamp          bool `yaml:"timestamp,omitempty" json:"timestamp,omitempty"`
	Keypress           bool `yaml:"keypress,omitempty" json:"keypress,omitempty"`
	Signal             bool `yaml:"signal,omitempty" json:"signal,omitempty"`
	Raw                bool `yaml:"raw,omitempty" json:"raw,omitempty"`
	Immediate          bool `yaml:"immediate,omitempty" json:"immediate,omitempty"`
	AutofocusOnCapture bool `yaml:"autofocus_on_capture,omitempty" json:"autofocus_on_capture,omitempty"`
	ZSL                bool `yaml:"zsl,omitempty" json:"zsl,omitempty"`

	// Integer values
	Camera                *int `yaml:"camera,omitempty" json:"camera,omitempty"`
	Width                 *int `yaml:"width,omitempty" json:"width,omitempty"`
	Height                *int `yaml:"height,omitempty" json:"height,omitempty"`
	Rotation              *int `yaml:"rotation,omitempty" json:"rotation,omitempty"`
	Shutter               *int `yaml:"shutter,omitempty" json:"shutter,omitempty"`
	EV                    *int `yaml:"ev,omitempty" json:"ev,omitempty"`
	Wrap                  *int `yaml:"wrap,omitempty" json:"wrap,omitempty"`
	ViewfinderWidth       *int `yaml:"viewfinder_width,omitempty" json:"viewfinder_width,omitempty"`
	ViewfinderHeight      *int `yaml:"viewfinder_height,omitempty" json:"viewfinder_height,omitempty"`
	LoresWidth            *int `yaml:"lores_width,omitempty" json:"lores_width,omitempty"`
	LoresHeight           *int `yaml:"lores_height,omitempty" json:"lores_height,omitempty"`
	BufferCount           *int `yaml:"buffer_count,omitempty" json:"buffer_count,omitempty"`
	ViewfinderBufferCount *int `yaml:"viewfinder_buffer_count,omitempty" json:"viewfinder_buffer_count,omitempty"`
	Framestart            *int `yaml:"framestart,omitempty" json:"framestart,omitempty"`
	Restart               *int `yaml:"restart,omitempty" json:"restart,omitempty"`
	Quality               *int `yaml:"quality,omitempty" json:"quality,omitempty"`
	Verbose               *int `yaml:"verbose,omitempty" json:"verbose,omitempty"`

	// Float values
	AnalogGain *float64 `yaml:"analog_gain,omitempty" json:"analog_gain,omitempty"`
	Gain       *float64 `yaml:"gain,omitempty" json:"gain,omitempty"`
	Brightness *float64 `yaml:"brightness,omitempty" json:"brightness,omitempty"`
	Contrast   *float64 `yaml:"contrast,omitempty" json:"contrast,omitempty"`
	Saturation *float64 `yaml:"saturation,omitempty" json:"saturation,omitempty"`
	Sharpness  *float64 `yaml:"sharpness,omitempty" json:"sharpness,omitempty"`
	Framerate  *float64 `yaml:"framerate,omitempty" json:"framerate,omitempty"`

	// String values
	InfoText        *string `yaml:"info_text,omitempty" json:"info_text,omitempty"`
	PostProcessFile *string `yaml:"post_process_file,omitempty" json:"post_process_file,omitempty"`
	PostProcessLibs *string `yaml:"post_process_libs,omitempty" json:"post_process_libs,omitempty"`
	Preview         *string `yaml:"preview,omitempty" json:"preview,omitempty"`
	PreviewLibs     *string `yaml:"preview_libs,omitempty" json:"preview_libs,omitempty"`
	ROI             *string `yaml:"roi,omitempty" json:"roi,omitempty"`
	Metering        *string `yaml:"metering,omitempty" json:"metering,omitempty"`
	Exposure        *string `yaml:"exposure,omitempty" json:"exposure,omitempty"`
	AWB             *string `yaml:"awb,omitempty" json:"awb,omitempty"`
	AWBGains        *string `yaml:"awb_gains,omitempty" json:"awb_gains,omitempty"`
	CCM             *string `yaml:"ccm,omitempty" json:"ccm,omitempty"`
	Denoise         *string `yaml:"denoise,omitempty" json:"denoise,omitempty"`
	TuningFile      *string `yaml:"tuning_file,omitempty" json:"tuning_file,omitempty"`
	Mode            *string `yaml:"mode,omitempty" json:"mode,omitempty"`
	ViewfinderMode  *string `yaml:"viewfinder_mode,omitempty" json:"viewfinder_mode,omitempty"`
	AutofocusMode   *string `yaml:"autofocus_mode,omitempty" json:"autofocus_mode,omitempty"`
	AutofocusRange  *string `yaml:"autofocus_range,omitempty" json:"autofocus_range,omitempty"`
	AutofocusSpeed  *string `yaml:"autofocus_speed,omitempty" json:"autofocus_speed,omitempty"`
	AutofocusWindow *string `yaml:"autofocus_window,omitempty" json:"autofocus_window,omitempty"`
	LensPosition    *string `yaml:"lens_position,omitempty" json:"lens_position,omitempty"`
	HDR             *string `yaml:"hdr,omitempty" json:"hdr,omitempty"`
	Metadata        *string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	MetadataFormat  *string `yaml:"metadata_format,omitempty" json:"metadata_format,omitempty"`
	EXIF            *string `yaml:"exif,omitempty" json:"exif,omitempty"`
	Thumb           *string `yaml:"thumb,omitempty" json:"thumb,omitempty"`
	Encoding        *string `yaml:"encoding,omitempty" json:"encoding,omitempty"`
	Latest          *string `yaml:"latest,omitempty" json:"latest,omitempty"`
	FlickerPeriod   *string `yaml:"flicker_period,omitempty" json:"flicker_period,omitempty"`
	Timeout         *string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Timelapse       *string `yaml:"timelapse,omitempty" json:"timelapse,omitempty"`
}

func appendFlag(args []string, flag string, val bool) []string {
	if val {
		return append(args, flag)
	}
	return args
}

func appendIntFlag(args []string, flag string, val *int) []string {
	if val != nil {
		return append(args, flag, strconv.Itoa(*val))
	}
	return args
}

func appendFloatFlag(args []string, flag string, val *float64) []string {
	if val != nil {
		return append(args, flag, strconv.FormatFloat(*val, 'f', -1, 64))
	}
	return args
}

func appendStringFlag(args []string, flag string, val *string) []string {
	if val != nil && *val != "" {
		return append(args, flag, *val)
	}
	return args
}

func ArgsFromConfig(cfg CameraConfig) []string {
	var args []string

	args = appendFlag(args, "--vflip", cfg.VFlip)
	args = appendFlag(args, "--hflip", cfg.HFlip)
	args = appendFlag(args, "-n", cfg.NoPreview)
	args = appendFlag(args, "-f", cfg.Fullscreen)
	args = appendFlag(args, "--qt-preview", cfg.QtPreview)
	args = appendFlag(args, "--flush", cfg.Flush)
	args = appendFlag(args, "--lores-par", cfg.LoresPAR)
	args = appendFlag(args, "--no-raw", cfg.NoRaw)
	args = appendFlag(args, "--datetime", cfg.Datetime)
	args = appendFlag(args, "--timestamp", cfg.Timestamp)
	args = appendFlag(args, "-k", cfg.Keypress)
	args = appendFlag(args, "-s", cfg.Signal)
	args = appendFlag(args, "-r", cfg.Raw)
	args = appendFlag(args, "--immediate", cfg.Immediate)
	args = appendFlag(args, "--autofocus-on-capture", cfg.AutofocusOnCapture)
	args = appendFlag(args, "--zsl", cfg.ZSL)

	args = appendIntFlag(args, "--camera", cfg.Camera)
	args = appendIntFlag(args, "--width", cfg.Width)
	args = appendIntFlag(args, "--height", cfg.Height)
	args = appendIntFlag(args, "--rotation", cfg.Rotation)
	args = appendIntFlag(args, "--shutter", cfg.Shutter)
	args = appendIntFlag(args, "--ev", cfg.EV)
	args = appendIntFlag(args, "--wrap", cfg.Wrap)
	args = appendIntFlag(args, "--viewfinder-width", cfg.ViewfinderWidth)
	args = appendIntFlag(args, "--viewfinder-height", cfg.ViewfinderHeight)
	args = appendIntFlag(args, "--lores-width", cfg.LoresWidth)
	args = appendIntFlag(args, "--lores-height", cfg.LoresHeight)
	args = appendIntFlag(args, "--buffer-count", cfg.BufferCount)
	args = appendIntFlag(args, "--viewfinder-buffer-count", cfg.ViewfinderBufferCount)
	args = appendIntFlag(args, "--framestart", cfg.Framestart)
	args = appendIntFlag(args, "--restart", cfg.Restart)
	args = appendIntFlag(args, "-q", cfg.Quality)
	args = appendIntFlag(args, "-v", cfg.Verbose)

	args = appendFloatFlag(args, "--analoggain", cfg.AnalogGain)
	args = appendFloatFlag(args, "--gain", cfg.Gain)
	args = appendFloatFlag(args, "--brightness", cfg.Brightness)
	args = appendFloatFlag(args, "--contrast", cfg.Contrast)
	args = appendFloatFlag(args, "--saturation", cfg.Saturation)
	args = appendFloatFlag(args, "--sharpness", cfg.Sharpness)
	args = appendFloatFlag(args, "--framerate", cfg.Framerate)

	args = appendStringFlag(args, "--info-text", cfg.InfoText)
	args = appendStringFlag(args, "--post-process-file", cfg.PostProcessFile)
	args = appendStringFlag(args, "--post-process-libs", cfg.PostProcessLibs)
	args = appendStringFlag(args, "-p", cfg.Preview)
	args = appendStringFlag(args, "--preview-libs", cfg.PreviewLibs)
	args = appendStringFlag(args, "--roi", cfg.ROI)
	args = appendStringFlag(args, "--metering", cfg.Metering)
	args = appendStringFlag(args, "--exposure", cfg.Exposure)
	args = appendStringFlag(args, "--awb", cfg.AWB)
	args = appendStringFlag(args, "--awbgains", cfg.AWBGains)
	args = appendStringFlag(args, "--ccm", cfg.CCM)
	args = appendStringFlag(args, "--denoise", cfg.Denoise)
	args = appendStringFlag(args, "--tuning-file", cfg.TuningFile)
	args = appendStringFlag(args, "--mode", cfg.Mode)
	args = appendStringFlag(args, "--viewfinder-mode", cfg.ViewfinderMode)
	args = appendStringFlag(args, "--autofocus-mode", cfg.AutofocusMode)
	args = appendStringFlag(args, "--autofocus-range", cfg.AutofocusRange)
	args = appendStringFlag(args, "--autofocus-speed", cfg.AutofocusSpeed)
	args = appendStringFlag(args, "--autofocus-window", cfg.AutofocusWindow)
	args = appendStringFlag(args, "--lens-position", cfg.LensPosition)
	args = appendStringFlag(args, "--hdr", cfg.HDR)
	args = appendStringFlag(args, "--metadata", cfg.Metadata)
	args = appendStringFlag(args, "--metadata-format", cfg.MetadataFormat)
	args = appendStringFlag(args, "-x", cfg.EXIF)
	args = appendStringFlag(args, "--thumb", cfg.Thumb)
	args = appendStringFlag(args, "-e", cfg.Encoding)
	args = appendStringFlag(args, "--latest", cfg.Latest)
	args = appendStringFlag(args, "--flicker-period", cfg.FlickerPeriod)
	args = appendStringFlag(args, "-t", cfg.Timeout)
	args = appendStringFlag(args, "--timelapse", cfg.Timelapse)

	return args
}
