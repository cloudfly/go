package ormx

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CacheProvider caches records queried by GetByID.
type CacheProvider interface {
	Get(table string, id any) ([]byte, bool)
	Set(ttl time.Duration, table string, id any, content []byte)
	Remove(table string, id any)
	RemoveAll(table string)
}

// DBProvider returns the master or slave database connection.
type DBProvider func(isMaster bool) *sql.DB

var (
	gopt *Option = &Option{}
)

// Init sets the global option used by all sql executions.
func InitGlobalOption(opt *Option) error {
	if opt == nil {
		gopt = &Option{}
	} else {
		gopt = opt.Copy()
	}
	return nil
}

func convertValueByDBType(v any, tag string) any {
	switch tag {
	case "timestamp":
		if t, ok := Any2Time(v); ok {
			return t
		}
	}
	return v
}

// Option for select or insert data
//
// You should use Opt() to initialize a new option, it will inherit the global option.
// DO NOT use Option{} directly, except you know what you are doing.
type Option struct {
	table         string
	tablePrefix   string
	fields        []string
	sorts         []string
	page          int
	pageSize      int
	primaryKey    string
	tagName       string
	fromMaster    bool
	tx            *Tx
	cacheProvider CacheProvider
	cacheTTL      time.Duration
	dbProvider    DBProvider
}

type optionCtxKey struct{}

// Opt try to get option from context, if not found, return global option will be returned
//
// The return value is a copy of target option, you can modify it safely.
func Opt(ctx context.Context) *Option {
	if ctx != nil {
		v := ctx.Value(optionCtxKey{})
		if v != nil {
			return v.(*Option).Copy()
		}
	}
	return gopt.Copy()
}

func (opt *Option) Copy() *Option {
	if opt == nil {
		return nil
	}
	newOpt := *opt
	newOpt.fields = append([]string{}, opt.fields...)
	newOpt.sorts = append([]string{}, opt.sorts...)

	return &newOpt
}

func (opt *Option) Table(value any) *Option {
	switch data := value.(type) {
	case string:
		opt.table = data
	case interface{ Table() string }:
		opt.table = data.Table()
	}
	return opt
}

func (opt *Option) Fields(value ...string) *Option {
	opt.fields = value
	return opt
}

func (opt *Option) Sorts(value ...string) *Option {
	opt.sorts = value
	return opt
}

func (opt *Option) Page(page, pageSize int) *Option {
	opt.page, opt.pageSize = page, pageSize
	return opt
}

func (opt *Option) PrimaryKey(value string) *Option {
	opt.primaryKey = value
	return opt
}

func (opt *Option) TagName(value string) *Option {
	opt.tagName = value
	return opt
}

func (opt *Option) TablePrefix(value string) *Option {
	opt.tablePrefix = value
	return opt
}

func (opt *Option) Tx(tx *Tx) *Option {
	opt.tx = tx
	return opt
}

func (opt *Option) GetTx() *Tx {
	if opt == nil {
		return nil
	}
	return opt.tx
}

func (opt *Option) FromMaster() *Option {
	opt.fromMaster = true
	return opt
}

func (opt *Option) FromSlave() *Option {
	opt.fromMaster = false
	return opt
}

// CacheProvider sets the cache used by GetByID, nil disables cache.
func (opt *Option) CacheProvider(provider CacheProvider) *Option {
	opt.cacheProvider = provider
	return opt
}

// DBProvider sets the required provider used to get the master or slave database connection.
func (opt *Option) DBProvider(provider DBProvider) *Option {
	opt.dbProvider = provider
	return opt
}

func (opt *Option) db(isMaster bool) (*DB, error) {
	if opt == nil || opt.dbProvider == nil {
		return nil, fmt.Errorf("ormx: DBProvider is required")
	}
	return NewDB(opt.dbProvider(isMaster)), nil
}

// With put the Option into context by Context.WithValue
//
// If the Option was shared among multiple goroutines, you should use Option.Copy() to create a copy of it before With().
//
//	opt.Copy().With(ctx)
func (opt *Option) With(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, optionCtxKey{}, opt)
}
