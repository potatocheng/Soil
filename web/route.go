package web

import (
	"fmt"
	"regexp"
	"strings"
)

// 路由优先级: 静态匹配 > 正则匹配 > 路径参数 > 通配符匹配
type node struct {
	// 该节点的路径
	path string

	// 静态匹配，用户注册的路由和请求的路径必须完全一致
	// string表示当前子节点的path
	children map[string]*node

	// 通配符匹配，使用符号* 进行匹配，*在路由匹配中可以匹配任意值
	// 由于路径都是*, 所以没必要记录path
	starChild *node

	// 路径参数，如当用户注册路由为/user/:id, 如果请求路径是 /user/123, 那么路径参数就是(id, path)
	paramChild *node
	paramName  string

	//正则匹配, 如/a/:key(regex)
	regexChild *node
	regExpr    *regexp.Regexp

	handler HandleFunc
}

// childOrCreate 查找n节点的子节点(children, starChild, paramChild, regexChild)，如果没有子节点就创建
// 同一个位置只能注册路径参数，通配符路由和正则路由中的一个。也就是三者是互斥的；
func (n *node) childOrCreate(path string) *node {
	//处理通配符子节点
	if path == "*" {
		if n.paramChild != nil {
			panic(fmt.Sprintf("web: 非法路由，已有路径参数路由。不允许同时注册通配符路由和参数路由 [%s]", path))
		}
		if n.regexChild != nil {
			panic(fmt.Sprintf("web: 非法路由，已有正则路由。不允许同时注册通配符路由和正则路由 [%s]", path))
		}

		if n.starChild == nil {
			//原来没有通配符路由，现在创建
			n.starChild = &node{path: path}
		}

		return n.starChild
	}

	if path[0] == ':' {
		//这里是路径参数和正则匹配存在两种情况
		param, regex, ok := n.parseParam(path)
		if !ok {
			//不是正则表达式,是路径参数
			return n.childOrCreateParam(path, param)
		}

		//是正则表达式
		return n.childOrCreateRegex(path, regex, param)
	}

	if n.children == nil {
		//该节点还没有子路由树，创建子路由树
		n.children = make(map[string]*node)
	}

	child, ok := n.children[path]
	if !ok {
		//没有匹配成功，创建节点
		child = &node{path: path}
		n.children[path] = child
	}

	return child
}

// parseParam 判断是不是正则表达式
// 第一个返回值表示参数名字
// 第二个返回值表示正则表达式
// 第三个返回值表示该path是不是正则路由
func (n *node) parseParam(path string) (string, string, bool) {
	//假如传入path = :id([0-9]+)
	path = path[1:]                          //去掉:, path = id([0-9]+)
	segments := strings.SplitN(path, "(", 2) //segments = [id, [0-9]+)]
	if len(segments) == 2 {
		exp := segments[1]
		if strings.HasSuffix(segments[1], ")") {
			return segments[0], exp[:len(exp)-1], true
		}
	}

	return path, "", false
}

func (n *node) childOrCreateParam(path string, paramName string) *node {
	if n.starChild != nil {
		panic(fmt.Sprintf("web: 非法路由，已有通配符路由。不允许同时注册通配符路由和参数路由 [%s]", path))
	}

	if n.regexChild != nil {
		panic(fmt.Sprintf("web: 非法路由，已有正则路由。不允许同时注册通配符路由和正则路由 [%s]", path))
	}

	if n.paramChild == nil {
		n.paramChild = &node{path: path, paramName: paramName}
	} else {
		if n.paramChild.path != path {
			panic(fmt.Sprintf("web: 非法路由，已有参数。已有[%s]， 新建[%s]", n.paramChild.path, path))
		}
	}

	return n.paramChild
}

func (n *node) childOrCreateRegex(path string, exp string, paramName string) *node {
	if n.starChild != nil {
		panic(fmt.Sprintf("web: 非法路由，已有通配符路由。不允许同时注册正则路由和通配符路由 [%s]", path))
	}

	if n.paramChild != nil {
		panic(fmt.Sprintf("web: 非法路由，已有参数路由。不允许同时注册正则路由和参数路由 [%s]", path))
	}

	if n.regexChild == nil {
		regExpr, err := regexp.Compile(exp)
		if err != nil {
			panic(fmt.Errorf("web: 正则表达式[%s]错误 %w", exp, err))
		}
		n.regexChild = &node{path: path, paramName: paramName, regExpr: regExpr}
	} else {
		if n.regExpr.String() != exp || n.paramName != paramName {
			panic(fmt.Sprintf("web: 路由冲突，正则路由冲突，已有 %s, 新注册 %s", n.regexChild.path, path))
		}
	}

	return n.regexChild
}

func (n *node) childOf(path string) (*node, bool)  {
	if n.children == nil {
		return n.childOfNonStatic(path)
	}

	//先进行静态查找，即在 children 中查找
	child, ok := n.children[path]
	if !ok {
		return n.childOfNonStatic(path)
	}

	return child, ok;
}

func (n *node) childOfNonStatic(path string) (*node, bool)  {
	if n.regexChild != nil {
		if n.regexChild.regExpr.Match([]byte(path)) {
			return n.regexChild, true
		}
	}

	if n.paramChild != nil {
		return n.paramChild, true
	}

	return n.starChild, n.starChild != nil
}

type router struct {
	// trees map<http.method, node>
	trees map[string]*node
}

func newRouter() *router {
	return &router{
		trees: make(map[string]*node),
	}
}

// addRoute 注册路由
// 路由设计：路由必须以 / 开头， 并且必须不能以 / 结尾; 在路由中不能出现 //这种情况
func (r *router) addRoute(method string, path string, handler HandleFunc) {
	if path == "" {
		panic("web: empty path")
	}

	if path[0] != '/' {
		panic("web: path must begin with '/'")
	}

	if path != "/" && path[len(path)-1] == '/' {
		panic("web: path must not end with '/'")
	}

	root, ok := r.trees[method]
	//该http方法还没有注册路由树
	if !ok {
		root = &node{path: "/"}
		r.trees[method] = root
	}

	if path == "/" {
		//注册根节点
		if root.handler != nil {
			panic("web: root already has a handler")
		}
		root.handler = handler
		return
	}

	segments := strings.Split(path[1:], "/")
	for _, segment := range segments {
		if segment == "" {
			panic(fmt.Sprintf("path[%s] invaild, 在路由中不能出现 //这种情况", path))
		}
		root = root.childOrCreate(segment)
	}

	if root.handler != nil {
		//路由冲突
		panic("web: routing conflict")
	}

	root.handler = handler
}

// findRoute 查找路由，在这里了体现路由优先级 静态匹配 > 正则匹配 > 路径参数 > 通配符匹配
func (r *router) findRoute(method string, path string) (*matchInfo, bool) {
	root, ok := r.trees[method]
	if !ok {
		return nil, false
	}


	if root.children != nil {
		segments := strings.Split(strings.Trim(path, "/"), "/")
		for _, seg := segments {

		}
	}
}

// matchInfo node表示匹配上的节点，匹配到children, starChild, regexChild时返回node;
// paramChild 表示匹配到路径参数，返回的内容类似<id:123>
type matchInfo struct {
	node       *node
	paramChild map[string]string
}

//func (m *matchInfo) addValue()  {
//
//}
