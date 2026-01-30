package watchf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/cloudfly/go/str"
)

// Watcher watch the changes of files in given directories, and call the given handler for each files.
type Watcher struct {
	// dirs represents the target directories would be watched
	dirs []string
	// logger for print useful message while watching directories
	logger func(string, error)
	// fileHandler handle the file found by Watcher in directories
	fileHandler func(File)
	// fileNameFilter filter the file by name. The file will be ignored if fileNameFilter return false.
	fileNameFilter func(string) bool
	// templateVars provide the variables which used to execute template
	templateVars any
	// templateFuncs customize the functions which can be used in template
	templateFuncs map[string]any
	// disableTemplate disable treating content in file as template, read it directly
	disableTemplate bool
	// interval specifiy the peroid cycle of detecting directories
	interval time.Duration
	files    sync.Map
	// context and cancel used to stop Watch()
	ctx    context.Context
	cancel func()
}

// New create a new Watcher for given directories
func New(dirs []string, opts ...Option) *Watcher {
	w := &Watcher{
		dirs:     dirs,
		interval: time.Second * 10,
		ctx:      context.Background(),
	}
	for _, opt := range opts {
		opt(w)
	}
	if w.templateFuncs == nil {
		w.templateFuncs = make(map[string]any)
	}
	for name, f := range builtinTemplateFuncs {
		w.templateFuncs[name] = f
	}
	return w
}

// Watch start to watch directories, it will block until Watcher.Stop() was invoked
//
// Watcher can be reused after it, it means that you can Watch() again after it was Stopped
func (w *Watcher) Watch() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var ctx context.Context
	ctx, w.cancel = context.WithCancel(w.ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.load()
		}
	}
}

// Stop the watcher
func (w *Watcher) Stop() {
	w.cancel()
}

func (w *Watcher) log(msg string, err error) {
	if w.logger == nil {
		return
	}
	w.logger(msg, err)
}

func (w *Watcher) isTargetFile(filename string) bool {
	if w.fileNameFilter == nil {
		return true
	}
	return w.fileNameFilter(filename)
}

func (w *Watcher) handleFile(f *File) {
	if w.fileHandler == nil {
		return
	}
	v, ok := w.files.Load(f.Path)
	if ok && v.(*File).UpdateTime.Equal(f.UpdateTime) {
		return
	}
	w.fileHandler(*f)
}

// GetJobs get job data from local configuration directory
func (w *Watcher) load() {
	for _, dir := range w.dirs {
		d, err := os.Open(dir)
		if err != nil {
			if !os.IsNotExist(err) && !os.IsPermission(err) {
				w.log(fmt.Sprintf("Failed to open directory %s, ignore it", dir), err)
			}
			continue
		}
		defer d.Close()
		names, err := d.Readdirnames(0)
		if err != nil {
			w.log(fmt.Sprintf("ERR: failed read files in dir %s", dir), err)
			continue
		}

		for _, name := range names {
			if !w.isTargetFile(name) {
				continue
			}
			p := path.Join(dir, name)

			info, err := os.Stat(p)
			if err != nil {
				w.log(fmt.Sprintf("Failed to read file stat of %s", p), err)
				continue
			}
			if info.IsDir() {
				continue
			}
			f, err := w.loadFile(p)
			if err != nil {
				w.log(fmt.Sprintf("Failed to load job from file %s", p), err)
				continue
			}
			w.handleFile(f)
		}
	}
}

func (w *Watcher) loadFile(filepath string) (*File, error) {
	// read and parse config file template
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("open file error: %w", err)
	}
	defer f.Close()

	finfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("read file stat error: %w", err)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read file error: %w", err)
	}

	if !w.disableTemplate {
		tmpl, err := template.New(path.Base(filepath)).Funcs(w.templateFuncs).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse template error: %s", err)
		}
		buf := &bytes.Buffer{}
		if err := tmpl.Execute(buf, w.templateVars); err != nil {
			return nil, fmt.Errorf("exeute template error: %s", err)
		}
		content = bytes.TrimSpace(buf.Bytes())
	}

	file := &File{
		Path:       filepath,
		Content:    string(content),
		UpdateTime: finfo.ModTime(),
	}

	return file, nil
}

var (
	builtinTemplateFuncs = map[string]interface{}{
		"now": func(layout ...string) string {
			format := time.RFC3339
			if len(layout) > 0 && layout[0] != "" {
				format = layout[0]
			}
			return time.Now().Format(format)
		},
		"split": func(value any, sep string) []string {
			s, ok := value.(string)
			if !ok || s == "" {
				return []string{}
			}
			if sep == "" {
				return []string{s}
			}
			return strings.Split(s, sep)
		},
		"splitAny": func(value any, sep string) []string {
			s, ok := value.(string)
			if !ok || s == "" {
				return []string{}
			}
			if sep == "" {
				return []string{s}
			}
			return strings.FieldsFunc(s, func(r rune) bool {
				return strings.ContainsRune(sep, r)
			})
		},
		"contain": str.Contain,
		"empty": func(value any) bool {
			return value == nil || reflect.ValueOf(value).IsZero()
		},
		"incr": func(value int) int {
			return value + 1
		},
		"seq": func(max int) []struct{} {
			return make([]struct{}, max)
		},
		"getenv": func(name string) string {
			return os.Getenv(name)
		},
	}
)

// File represent a file found by Watcher in directories
type File struct {
	Path       string
	Content    string
	UpdateTime time.Time
	HandleTime time.Time
}

// Option for watcher
type Option func(*Watcher)

// WithLogger customize the logger function
func WithLogger(f func(string, error)) Option {
	return func(w *Watcher) {
		w.logger = f
	}
}

// WithTemplateVars customize the variables used to parse template
func WithTemplateVars(data any) Option {
	return func(w *Watcher) {
		w.templateVars = data
	}
}

// WithTemplateVars customize the functions used to parse template
func WithTemplateFuncs(fs map[string]any) Option {
	return func(w *Watcher) {
		w.templateFuncs = fs
	}
}

// WithTemplateVars set the file handler for watcher. It will be called will file was changed or first found.
func WithHandler(h func(File)) Option {
	return func(w *Watcher) {
		w.fileHandler = h
	}
}

// WithFileNameFilter used to ignore some files in directories. such as some big file.
func WithFileNameFilter(f func(string) bool) Option {
	return func(w *Watcher) {
		w.fileNameFilter = f
	}
}

// WithInterval set the interval for detecting changes of files. 10 seconds by default.
func WithInterval(d time.Duration) Option {
	return func(w *Watcher) {
		w.interval = d
	}
}

// WithContext set a context for watcher. watcher will automatically stopped when context was canceled
func WithContext(ctx context.Context) Option {
	return func(w *Watcher) {
		w.ctx = ctx
	}
}

// WithDisableTemplate disable the template execution, watcher will parse the raw content in file to the Handler
func WithDisableTemplate(b bool) Option {
	return func(w *Watcher) {
		w.disableTemplate = b
	}
}
