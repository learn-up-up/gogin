package gogin

import (
	"math"
	"net/http"
	"html/template"
	"github.com/julienschmidt/httprouter"
	"path"
	"log"
	"encoding/json"
)

const (
	AbortIndex = math.MaxInt8 /2
)

type (
	HandlerFunc func(*Context)

	H map[string]interface{}

	ErrorMsg struct {
		Message string		`json:"msg"`
		Meta	interface{}	`json:"meta"`
	}

	ResponseWriter interface {
		http.ResponseWriter
		Status() int
		Written() bool
	}

	responseWriter struct {
		http.ResponseWriter
		status int
	}

	Context struct {
		Req		*http.Request
		Writer	ResponseWriter
		Keys	map[string]interface{}
		Errors	[]ErrorMsg
		Params	httprouter.Params
		handlers []HandlerFunc
		engine 	*Engine
		index	int8
	}

	RouterGroup struct {
		Handlers []HandlerFunc
		prefix   string
		parent   *RouterGroup
		engine 	 *Engine
	}

	Engine struct {
		*RouterGroup
		handlers404 	[]HandlerFunc
		router 			*httprouter.Router
		HTMLTemplates 	*template.Template
	}
)

func (rw *responseWriter) WriteHeader(s int) {
	rw.ResponseWriter.WriteHeader(s)
	rw.status = s
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Status() int  {
	return rw.status
}

func (rw *responseWriter) Written() bool  {
	return rw.status != 0
}


func (engine *Engine) LoadHTMLTemplates(pattern string) {
	engine.HTMLTemplates = template.Must(template.ParseGlob(pattern))
}

func (engine *Engine) NotFound404(handles ... HandlerFunc) {
	engine.handlers404 = handles
}

func (engine *Engine) ServeFiles(path string, root http.FileSystem) {
	engine.router.ServeFiles(path, root)
}

func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	engine.router.ServeHTTP(w,req)

}

func (engine *Engine) Run(addr string) {
	http.ListenAndServe(addr, engine)
}

func(group * RouterGroup) createContext(w http.ResponseWriter, req *http.Request, params httprouter.Params,handles []HandlerFunc) *Context {
	return &Context{
		Writer:	&responseWriter{w,0},
		Req:	req,
		index:	-1,
		engine: group.engine,
		Params: params,
		handlers:handles,
	}
}

func (group *RouterGroup) allHandlers(handles []HandlerFunc) []HandlerFunc {
	local := append(group.Handlers, handles...)
	if group.parent != nil {
		return group.parent.allHandlers(local)
	} else {
		return local
	}
}

func (engine *Engine) handle404(w http.ResponseWriter, req *http.Request) {
	handlers := engine.allHandlers(engine.handlers404)
	c := engine.createContext(w, req, nil, handlers)
	//c.Next()
	if !c.Writer.Written() {
		http.NotFound(c.Writer, c.Req)
	}
}

func (group *RouterGroup) Use(middlewares ...HandlerFunc) {
	group.Handlers = append(group.Handlers, middlewares...)
}

func New() *Engine {
	engine := &Engine{}
	engine.RouterGroup = &RouterGroup{nil,"/",nil,engine}
	engine.router = httprouter.New()
	engine.router.NotFound.ServeHTTP = engine.handle404
	return engine
}

func Default() *Engine {
	engine := New()
	engine.Use(nil)
	return engine
}

func (group *RouterGroup) Group(component string, handlers...HandlerFunc) *RouterGroup {
	prefix := path.Join(group.prefix, component)
	return  &RouterGroup{
		Handlers: handlers,
		parent:	group,
		prefix: prefix,
		engine: group.engine,
	}
}

func (c *Context) Next() {
	c.index++
	s := int8(len(c.handlers))
	for ; c.index < s; c.index++ {
		c.handlers[c.index](c)
	}
}

func (group *RouterGroup) Handle(method, p string, handlers []HandlerFunc) {
	p = path.Join(group.prefix, p)
	handlers = group.allHandlers(handlers)
	group.engine.router.Handle(method, p, func(w http.ResponseWriter, req *http.Request, params httprouter.Params){
		group.createContext(w, req, params, handlers).Next()
	})
}

func (group *RouterGroup) POST(path string, handlers ...HandlerFunc) {
	group.Handle("POST", path, handlers)
}

func (group *RouterGroup) GET(path string, handlers ...HandlerFunc) {
	group.Handle("GET", path, handlers)
}

func (group *RouterGroup) DELETE(path string, handlers ...HandlerFunc)  {
	group.Handle("DELETE", path, handlers)
}

func (group *RouterGroup) PATCH(path string, handlers ...HandlerFunc) {
	group.Handle("PATCH", path, handlers)
}

func (group *RouterGroup) PUT(path string, handlers ...HandlerFunc) {
	group.Handle("PUT", path, handlers)
}

func (c *Context) Abort(code int) {
	c.Writer.WriteHeader(code)
	c.index = AbortIndex
}

func (c *Context) Error(err error, meta interface{}) {
	c.Errors = append(c.Errors, ErrorMsg{
		Message: err.Error(),
		Meta: meta,
	})
}

func (c *Context) Fail(code int, err error) {
	c.Error(err,"Operation aborted")
	c.Abort(code)
}

func (c *Context) Set(key string, item interface{}) {
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}
	c.Keys[key] = item
}

func (c *Context) Get(key string) interface{} {
	var ok bool
	var item interface{}
	if c.Keys != nil {
		item, ok = c.Keys[key]
	} else {
		item, ok = nil, false
	}
	if !ok || item == nil {
		log.Panicf("Key %s doesn't exist",key)
	}
	return item
}

func (c *Context) ParseBody(item interface{}) error {
	decoder := json.NewDecoder(c.Req.Body)
	if err := decoder.Decode(&item); err ==nil {
		return Validate(c, item)
	} else {
		return err
	}
}

func (c *Context) EnsureBody(item interface{}) bool {
	if err := c.ParseBody(item); err != nil {
		c.Fail(400, err)
	}
}