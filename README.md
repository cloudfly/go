## pkg

**pkg** includes a series of golang library which can accelerate your developlment on golang project. 

It provide many of convinient functions to simplify your code; besides, it has no any third part dependencies.

## libs

### Basic Types
- `str`: functions for handling string values
- `types`: generic functions for handling any types
- `floats`: generic functions for handling any types
- `deepcopy`: copy values deeply
- `bytesutil`: bytes buffer and some functions for handling `[]byte` type of data.

### Time

- `timerpool`: pool for timer
- `timelimiter`: qps limiter
- `fasttime`: high performance function to get current unix timestamp
- `duration`: convinient functions for handle time.Duration

### Limiter
- `timelimiter`: qps limiter
- `pacelimiter`: qps limiter
- `concurrencylimiter`: tool for limiting the concurrency


### Tools
- `logid`: generate unique id string, which can be used as trace id
- `urls`: functions to handle url string
- `watchf`: watch the changes of local files in given directories.
- `validator`: rules to validate values.
- `env`: convinient functions intergrate with environment values.
- `cgroup`: get cpu cores and memory limitation infomation.
- `simpletmpl`: a very simple template tool for templating short string.
