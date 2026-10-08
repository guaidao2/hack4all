---
id: lang-java-jsp
title_en: "Java and JSP — Objects, Reflection and the Servlet Parsers"
title_zh: "Java 与 JSP：对象、反射与 Servlet 的解析器"
summary_en: Java treats objects and bytes as interchangeable, which is why deserialisation is a language feature rather than a library call, and it hands the application several different notions of the request path. This entry covers the type-system surprises, reflection as a string-to-class bridge, what deserialisation actually invokes and the three parts of a gadget chain, expression language, and the Servlet path functions that disagree with each other.
summary_zh: Java 把对象与字节视为可互换，这就是为什么反序列化是一个语言特性而不是一次库调用；而它又交给应用好几个不同的"请求路径"概念。这一篇讲类型系统里的意外、把字符串变成类的反射、反序列化到底会调用什么以及 gadget 链的三个部分、表达式语言，以及那几个互相不一致的 Servlet 路径函数。
tags: [web, java, jsp, deserialization-prep, reflection, servlet]
tools: [java, ysoserial, Burp Suite, curl, javac]
attck: [T1190, T1059.006]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why Java is its own entry

Three properties make this runtime distinct, and all three are language-level rather than framework-level:

1. **Objects are bytes and bytes are objects.** `Serializable` is a marker interface, `ObjectOutputStream` writes an object graph to a stream, and `ObjectInputStream` rebuilds it — with side effects. Deserialisation is not a library you opted into; it is part of the platform.
2. **Reflection makes strings into classes.** `Class.forName(name)` and `Method.invoke` mean a string in the wrong place becomes a constructor call.
3. **The request path has several definitions.** `getRequestURI`, `getServletPath`, `getPathInfo` and `getContextPath` are all "the path", and containers disagree about normalisation and decoding. This is the same class of bug as everything in this series, expressed inside one API.

### Type-system surprises

Java is statically typed, and these are the places where that is less protective than it looks.

**Boxed types and `==`.** The classic trap, and it is a memory-cache artefact rather than an accident:

```java
Integer a = 127, b = 127;
System.out.println(a == b);      // true  — both come from the Integer cache (-128..127)
Integer c = 128, d = 128;
System.out.println(c == d);      // false — outside the cache, two distinct objects
System.out.println(c.equals(d)); // true  — the comparison you actually wanted
```

Code that compares identifiers, roles or tokens with `==` on boxed values is correct for small numbers and wrong for large ones, which is the worst kind of bug: it passes the developer's test with `id = 1` and fails in production with `id = 1000`. The rule is simple — **`equals` for objects, `==` only for primitives** — and it is worth checking specifically when a security decision compares a boxed value.

**Integer overflow is silent.** Java wraps without complaint:

```java
System.out.println(Integer.MAX_VALUE + 1 == Integer.MIN_VALUE);   // true
int len = Integer.MAX_VALUE; len += 1;                            // -2147483648, no exception
```

A length check like `if (offset + length > buffer.length)` can therefore be defeated with values that sum past `Integer.MAX_VALUE`. The fix is to widen the arithmetic or check the bound differently — `if (offset > buffer.length - length)` — which is the same reasoning as the integer section in the encoding entries.

**Type erasure.** Generics exist only at compile time:

```java
List<String> list = new ArrayList<>();
System.out.println(list.getClass().getName());   // java.util.ArrayList — no String anywhere
```

The consequence that matters here: **a generic type parameter cannot constrain what a deserialisation or a reflection call actually produces.** `List<String>` in the source is `List` at runtime, so code that trusts the declared type of data returning from a stream or a cast is trusting the compiler rather than the object.

**Primitive conversion is width-blind.** Assigning a `long` to an `int` truncates silently, and mixing `int` and `long` in arithmetic promotes in ways that surprise.

```java
long big = 0x1_0000_0001L;
int small = (int) big;            // 1 — the high bits are gone, no warning
```

### Strings, bytes, and where they disagree

| Feature | Behaviour | Why it matters |
|---|---|---|
| **`String.getBytes()`** | Uses the **platform default charset** when none is given | The same code produces different bytes on different hosts; always pass a charset explicitly |
| **`URLDecoder.decode`** | Decodes percent-encoding | Calling it twice is the double-decoding bug from the encoding entry |
| **Regular expressions** | The engine has **no timeout** and can backtrack exponentially | A user-supplied pattern, or a user-supplied *subject* against a careless pattern, is a denial-of-service primitive |
| **`String.intern()` and literals** | Identical literals are the same object | Makes `==` on strings "work" in testing and fail elsewhere, which is how the `==` habit survives |
| **Immutability** | Strings cannot be modified in place | Good for security; also why "sanitise the string" often does nothing — the original is still referenced |

```java
// the charset trap, and the fix
byte[] platform = "café".getBytes();                                  // depends on the host
byte[] utf8     = "café".getBytes(java.nio.charset.StandardCharsets.UTF_8);
System.out.println(java.util.Arrays.toString(platform));
System.out.println(java.util.Arrays.toString(utf8));
```

### Reflection: turning a string into a class

Reflection is what lets a string in the input decide **what code runs**, and the pattern to look for is short:

```java
// a string from a request becomes a class, then an instance
Object o = Class.forName(userInput).getDeclaredConstructor().newInstance();

// or a method name from a request is invoked
Method m = obj.getClass().getMethod(userInput);
m.invoke(obj);
```

Both are legitimate features — dependency injection, plugin loading, serialisation frameworks and web frameworks all use them — and both become code execution when the string is user-controlled and the classpath is rich. In practice the interesting cases are indirect: a framework that reflects on a parameter name, or a serialisation library that constructs the class named **inside the data**.

```java
// what the data-controlled version looks like conceptually
Class<?> c = Class.forName(classNameFromThePayload);
Object instance = c.getDeclaredConstructor().newInstance();   // constructors run
```

That last line is what makes deserialisation dangerous in a way that reading JSON is not.

### Deserialisation as a language feature

This is the bridge to the deserialisation entry, so the language-level facts belong here.

**What `Serializable` means.** It is a **marker interface** with no methods. Declaring it says "instances may be written to a stream and rebuilt later", and that is the entire contract the author signs. It is very easy to add, and its consequences are not obvious at the point where it is added.

**What the stream looks like.** A Java serialisation stream begins with a fixed signature, which is a detection gift:

```
AC ED 00 05
```

Base64-encoded, that is the string **`rO0AB...`**, so a payload carrying a serialised Java object is recognisable in an HTTP request, a cookie, a form field or a log line without decoding it first.

```java
ByteArrayOutputStream bos = new ByteArrayOutputStream();
try (ObjectOutputStream oos = new ObjectOutputStream(bos)) {
    oos.writeObject(new java.util.ArrayList<String>());
}
byte[] data = bos.toByteArray();
for (int i = 0; i < 4; i++) System.out.printf("%02X ", data[i]);   // AC ED 00 05
System.out.println();
System.out.println(java.util.Base64.getEncoder().encodeToString(data).substring(0, 8));  // rO0AB...
```

**What deserialisation actually invokes.** This is the part that makes it different from parsing data into a struct:

| Invoked during read | Notes |
|---|---|
| `readObject` | The class's own custom deserialisation method, if defined |
| `readResolve` | Lets the class substitute a different object for the one read |
| `readExternal` | For `Externalizable` classes |
| `validateObject` | When validation is enabled on the stream |
| **Constructors** | **Not invoked** — which is why an object can exist in a state its constructor would have prevented |

An object arriving from a stream **has never had its constructor run**, so any invariant enforced there is absent. Combined with the hooks above — which are ordinary methods that the class author wrote for other reasons — the read itself becomes a code path, and **the payload chooses which classes take it**.

**The gadget chain, in three parts.** This is the vocabulary the deserialisation entry will use, and it is worth having now:

1. **A source** — an entry point where untrusted bytes reach `readObject`. A cookie, a view state, a message body, an RMI call, a cache entry.
2. **A gadget** — a class on the classpath that does something dangerous as a side effect of being deserialised: a `readObject` that calls a method from the data, a `readResolve` that performs a lookup, a finaliser that runs a command.
3. **A sink** — the operation that actually matters: a command execution, a file write, a JNDI lookup, a URL fetch.

A chain is source → gadget → sink, and the reason the problem is structural is that **the gadget does not have to be part of the application's own code** — it only has to be on the classpath, which in a large Java application is hundreds of libraries.

**Blacklists lose, allowlists work.**

```java
// fragile: enumerate known-dangerous classes and hope the list stays complete
if (className.startsWith("org.apache.commons.collections")) { reject(); }

// reliable: name the classes the application actually needs
ObjectInputFilter filter = ObjectInputFilter.Config.createFilter(
    "com.example.app.dto.*;java.base/*;!*");
try (ObjectInputStream ois = new ObjectInputStream(input)) {
    ois.setObjectInputFilter(filter);
    return ois.readObject();
}
```

`ObjectInputFilter` (JDK 9+, JEP 290) is the mechanism; the ordering matters — `!*` at the end rejects everything not explicitly allowed. The pattern to avoid is the **deny-list of known gadget classes**, because it is a list of what has already been published, and the interesting gadgets are the ones that have not been.

**And the first rule outranks the mechanism**: do not deserialise untrusted data. If the data can be a documented format like JSON, use that — the entire class of bug comes from the platform's willingness to reconstruct arbitrary objects, and a format that cannot express "class name" cannot be talked into constructing one.

### JSP and expression language

**JSP compiles to a Servlet.** The `<% %>` scriptlets are Java source, which is why a JSP file is a code execution surface the moment its content is influenced.

**Expression language is an evaluator.** `${...}` is not a template placeholder — it is an expression evaluated by the container:

```jsp
${7*7}          <!-- 49 -->
${param.name}   <!-- reads a request parameter -->
${header.host}  <!-- reads a header -->
```

EL 2.2 and later also allow **method invocation** on the objects it can reach. So an application that places user input into an EL expression — for example by building a message template from a parameter — has handed an attacker an evaluator with access to request data and, depending on the implementation and the objects in scope, rather more.

```java
// the shape of the mistake: user input becomes part of an expression
String expr = "${" + userInput + "}";
// an attacker supplies something that reads what they want, not a name
```

**Output escaping is per-tag, not per-language.** `<%= value %>` writes as-is; `<c:out value="${value}"/>` escapes. The language does not decide; the tag does — which is the same statement as the ASP.NET entry's "escaping depends on the control".

**Inclusion comes in two forms**, and they behave differently:

| Directive | When it is included |
|---|---|
| `<%@ include file="..." %>` | At translation time — static, part of the compiled page |
| `<jsp:include page="..." />` | At request time — dynamic, a separate request to another resource |

### The Servlet path functions, and why they disagree

This is the most useful part of the entry for testing, because it is where the "two parsers" pattern is visible **inside a single API**.

| Method | Returns |
|---|---|
| `getRequestURI()` | The raw URI path, **not decoded**, including the context path |
| `getServletPath()` | The part mapped to the servlet, **decoded** by the container |
| `getPathInfo()` | The remainder after the servlet mapping, decoded |
| `getContextPath()` | The application's context prefix |
| `getQueryString()` | The raw query, undecoded |

Three consequences that show up in real vulnerabilities:

**1. A check on one and a use of another.** A filter that validates `getRequestURI()` and a servlet that routes on `getServletPath()` are looking at different strings, and the differences include decoding and normalisation:

```
/app/admin/../user/list      -> getRequestURI may keep "..", getServletPath may be normalised
/app/%2e%2e/admin            -> decoding decisions differ by container and configuration
/app/list;jsessionid=ABC     -> the container strips the path parameter; a naive check may not
```

**2. Path parameters.** Java containers support `;name=value` segments in a path, most commonly `;jsessionid=XYZ`. The container removes them before mapping; a security check that reads the raw URI sees them. That is a bypass primitive, and it is a language-and-container convention rather than a bug in either.

**3. Encoded separators.** Whether `%2f` inside a path is decoded before mapping — and therefore whether it becomes a separator — is a **container setting** (Tomcat's encoded-slash handling is the usual example). Two components with different settings disagree about how many segments a path has.

**The way to reason about it** is the same as everywhere in this series: list the components that see the URI (reverse proxy, connector, container mapper, filters, servlet, framework router), and ask what each one decodes and normalises. The vulnerability is where a decision is made by one and used by another.

### File upload in Java, at the language level

Upload handling is its own entry; the language facts are:

| API | Behaviour |
|---|---|
| **Servlet 3.0 `Part`** | `part.getSubmittedFileName()` returns the **client-supplied filename**; older `getName()` returned a full path in some containers, which is where traversal came from |
| **`commons-fileupload`** | `FileItem.getName()` also returns the client name, and the library's own guidance is to strip any path from it |
| **`MultipartFile` (Spring)** | `getOriginalFilename()` is the client name; the framework does not sanitise it |
| **`getContentType()`** | The **client-declared** type, in every one of these APIs |

The pattern is consistent with the PHP entry: **the filename and the content type come from the request**, so any check based on them is a check on attacker-supplied data, and the storage decision needs to be made on canonicalised server-side values.

### Recognising the stack

| Signal | What it tells you |
|---|---|
| `JSESSIONID` cookie, `;jsessionid=` in a URL | Servlet container |
| `Server: Apache-Coyote/1.1` | Tomcat's connector |
| `X-Powered-By: Servlet/3.1 JSP/2.3` | Servlet and JSP spec versions |
| `.jsp`, `.do`, `.action`, `.jsf` | JSP, Struts-style, JSF |
| Detailed stack traces with package names | Framework and library versions, and a rich classpath |
| `rO0AB` in a cookie, parameter or body | A Java serialisation stream, base64-encoded |

### Detection and mitigation

- **Do not deserialise untrusted data, and prefer a format that cannot express a class name.** Where Java serialisation is genuinely required, apply `ObjectInputFilter` with an **allowlist** of the classes the application needs, and treat any deny-list of known gadget classes as a temporary measure rather than a control.
- **Alert on the serialisation signature and on EL syntax in input.** `rO0AB` (and the raw bytes `AC ED 00 05`), `${`, `#{(` and `%24%7B` appearing in a parameter, cookie or body are structural anomalies: they are the platform's vocabulary appearing in a client's request. This is the same signal as `php://` in the PHP entry and `$`-prefixed keys in the NoSQL entry.
- **Expect a classpath problem, not an application problem.** The gadget does not have to be yours; it has to be somewhere on the classpath. That is why the finding is usually reported against the deserialisation entry point rather than against the library that supplied the gadget.
- **Compare boxed values with `equals`, and do not let a security decision rest on `==`.** The integer-cache behaviour means the bug is invisible in testing and present in production, which is exactly the kind of finding a code review should look for deliberately.
- **Widen arithmetic before comparing bounds, and validate the result rather than the inputs.** `offset + length > size` overflows; `offset > size - length` does not.
- **Pass a charset explicitly on every string-to-byte conversion.** `getBytes()` without an argument is a portability bug today and a security-relevant one whenever two components disagree about the encoding.
- **Treat the path as several strings, and validate the one you will act on.** Read the same accessor the routing uses, after the same normalisation, and remember that path parameters and encoded separators mean the raw URI and the mapped path can describe different resources.
- **Bound regular expressions, or keep them away from user input.** The engine has no timeout, so an unbounded pattern over attacker-controlled data is a denial-of-service primitive; the fix is a simpler pattern, a length bound on the input, or a length check before matching.
- **Log the accessors, not just the path.** Recording `getRequestURI`, `getServletPath` and `getPathInfo` side by side in a request log turns a class of routing bypass into something visible, because the disagreement is between two of those values rather than inside any one of them.

<!-- lang:zh -->
### 为什么 Java 值得单独一篇

有三个性质让这个运行时与众不同，而且三个都是**语言层面**的，不是框架层面的：

1. **对象就是字节，字节就是对象。** `Serializable` 是一个标记接口，`ObjectOutputStream` 把对象图写进流，`ObjectInputStream` 把它重建出来 —— 还带着副作用。反序列化不是你要主动引入的某个库；它是这个平台的一部分。
2. **反射把字符串变成类。** `Class.forName(name)` 和 `Method.invoke` 意味着**放错位置的字符串会变成一次构造器调用**。
3. **请求路径有好几个定义。** `getRequestURI`、`getServletPath`、`getPathInfo`、`getContextPath` 都是"那个路径"，而各容器对规范化和解码的意见不一致。这和本系列里其他每一篇是同一类 bug，只是体现在一个 API 内部。

### 类型系统里的意外

Java 是静态类型的，而下面这些地方，那份静态性没有看上去那么有保护力。

**包装类型与 `==`。** 经典陷阱，而且它是一个内存缓存的产物，不是意外：

```java
Integer a = 127, b = 127;
System.out.println(a == b);      // true  —— 两者都来自 Integer 缓存（-128..127）
Integer c = 128, d = 128;
System.out.println(c == d);      // false —— 超出缓存范围，是两个不同的对象
System.out.println(c.equals(d)); // true  —— 这才是你想要的比较
```

用 `==` 比较包装类型的标识符、角色或令牌的代码，对小的数是对的、对大的数是错的 —— 这是最糟的一类 bug：开发者用 `id = 1` 测试时通过，生产环境里 `id = 1000` 就失效。规则很简单 —— **对象用 `equals`，`==` 只用于基本类型** —— 而当某个安全决策比较的是包装值时，尤其值得专门去查。

**整数溢出是静默的。** Java 会直接回绕，不会有任何抱怨：

```java
System.out.println(Integer.MAX_VALUE + 1 == Integer.MIN_VALUE);   // true
int len = Integer.MAX_VALUE; len += 1;                            // -2147483648，无异常
```

于是像 `if (offset + length > buffer.length)` 这样的长度检查，可以被"和超过 `Integer.MAX_VALUE`"的取值击败。修法是把运算宽度放大、或者换一种方式检查边界 —— `if (offset > buffer.length - length)` —— 和编码那几篇里的整数部分同一个推理。

**类型擦除。** 泛型只存在于编译期：

```java
List<String> list = new ArrayList<>();
System.out.println(list.getClass().getName());   // java.util.ArrayList —— 哪儿都没有 String
```

在这里要紧的后果是：**泛型参数无法约束反序列化或反射调用真正产生什么。** 源码里的 `List<String>` 在运行时就是 `List`，所以那些相信"从流里回来的数据、或者被强转的对象、其声明类型是可靠的"的代码，相信的是编译器而不是那个对象。

**基本类型转换不看宽度。** 把 `long` 赋给 `int` 会静默截断，而在算术里混用 `int` 与 `long` 的提升方式也会给人惊喜。

```java
long big = 0x1_0000_0001L;
int small = (int) big;            // 1 —— 高位没了，也没有警告
```

### 字符串、字节，以及它们在哪里不一致

| 特性 | 行为 | 为什么重要 |
|---|---|---|
| **`String.getBytes()`** | 不给参数时用**平台默认字符集** | 同一段代码在不同主机上产出不同字节；永远显式传字符集 |
| **`URLDecoder.decode`** | 解码百分号编码 | 调两次就是编码那篇里的二次解码 bug |
| **正则表达式** | 引擎**没有超时**，可以指数级回溯 | 用户提供的模式，或者用户提供的数据配上一个不小心的模式，就是拒绝服务的原语 |
| **`String.intern()` 与字面量** | 相同字面量是同一个对象 | 这让字符串上的 `==` 在测试里"能work"、在别处失效 —— `==` 的习惯就是这么活下来的 |
| **不可变性** | 字符串无法原地修改 | 对安全是好事；也解释了为什么"净化字符串"常常什么也没做 —— 原对象仍被引用着 |

```java
// 字符集陷阱，以及修法
byte[] platform = "café".getBytes();                                  // 取决于主机
byte[] utf8     = "café".getBytes(java.nio.charset.StandardCharsets.UTF_8);
System.out.println(java.util.Arrays.toString(platform));
System.out.println(java.util.Arrays.toString(utf8));
```

### 反射：把字符串变成类

反射就是让输入里的一个字符串决定**跑什么代码**的东西，而要找的模式很短：

```java
// 一个来自请求的字符串变成类，再变成实例
Object o = Class.forName(userInput).getDeclaredConstructor().newInstance();

// 或者来自请求的方法名被调用
Method m = obj.getClass().getMethod(userInput);
m.invoke(obj);
```

两者都是正当功能 —— 依赖注入、插件加载、序列化框架和 Web 框架都在用 —— 而当那个字符串由用户控制、且类路径很丰富时，两者都变成代码执行。实践中有意思的往往是间接情形：某个框架对参数名做反射，或者某个序列化库构造**数据里写着名字的那个类**。

```java
// 数据可控的那种情形，概念上长这样
Class<?> c = Class.forName(classNameFromThePayload);
Object instance = c.getDeclaredConstructor().newInstance();   // 构造器会跑
```

最后那一行，正是反序列化比"读一段 JSON"危险的地方。

### 反序列化是一个语言特性

这是通向反序列化那一篇的桥，所以语言层面的事实放在这里。

**`Serializable` 意味着什么。** 它是一个**没有任何方法的标记接口**。声明它就等于说"实例可以被写进流、以后再重建出来"，而这就是作者签下的全部契约。它极容易加，而它的后果在加它的那个地方并不明显。

**这个流长什么样。** Java 序列化流以一个固定签名开头，这是一个检测上的礼物：

```
AC ED 00 05
```

Base64 编码之后就是 **`rO0AB...`** 这个前缀，所以一个携带 Java 序列化对象的载荷，在 HTTP 请求、cookie、表单字段或日志里**不先解码就能认出来**。

```java
ByteArrayOutputStream bos = new ByteArrayOutputStream();
try (ObjectOutputStream oos = new ObjectOutputStream(bos)) {
    oos.writeObject(new java.util.ArrayList<String>());
}
byte[] data = bos.toByteArray();
for (int i = 0; i < 4; i++) System.out.printf("%02X ", data[i]);   // AC ED 00 05
System.out.println();
System.out.println(java.util.Base64.getEncoder().encodeToString(data).substring(0, 8));  // rO0AB...
```

**反序列化实际会调用什么。** 这是它和"把数据解析成结构体"的区别所在：

| 读取期间被调用 | 说明 |
|---|---|
| `readObject` | 该类自定义的反序列化方法（若有定义） |
| `readResolve` | 允许该类用另一个对象替换刚读出来的这个 |
| `readExternal` | 用于 `Externalizable` 类 |
| `validateObject` | 在流上启用校验时 |
| **构造器** | **不会被调用** —— 所以一个对象可以存在于它的构造器本会阻止的状态里 |

从流里到达的对象**从来没有跑过构造器**，于是任何在那里建立的不变式都不存在。再加上上面那些钩子 —— 它们是类作者为别的目的写的普通方法 —— **读取这件事本身就成了一条代码路径，而载荷决定了哪些类会走上它**。

**gadget 链，三个部分。** 这是反序列化那一篇会用的词汇，现在就该有：

1. **入口（source）** —— 不可信字节到达 `readObject` 的地方。cookie、view state、消息体、RMI 调用、缓存条目。
2. **gadget** —— 类路径上某个"在被反序列化时会顺带做危险事"的类：一个会调用数据里指定方法的 `readObject`、一个会做查询的 `readResolve`、一个会执行命令的终结器。
3. **落点（sink）** —— 真正要紧的那个操作：命令执行、写文件、JNDI 查询、URL 抓取。

一条链就是 入口 → gadget → 落点，而这个问题之所以是结构性的，原因在于**gadget 不必是应用自己的代码** —— 它只需要在类路径上，而一个大型 Java 应用的类路径上有几百个库。

**黑名单会输，白名单才管用。**

```java
// 脆弱：枚举已知危险的类，然后祈祷这份清单是完整的
if (className.startsWith("org.apache.commons.collections")) { reject(); }

// 可靠：点明应用真正需要的类
ObjectInputFilter filter = ObjectInputFilter.Config.createFilter(
    "com.example.app.dto.*;java.base/*;!*");
try (ObjectInputStream ois = new ObjectInputStream(input)) {
    ois.setObjectInputFilter(filter);
    return ois.readObject();
}
```

`ObjectInputFilter`（JDK 9+，JEP 290）是机制；顺序要紧 —— 末尾的 `!*` 拒绝所有未被显式允许的东西。要避开的模式是**"已知 gadget 类的黑名单"**，因为它列的是已经被公开的东西，而真正有意思的 gadget 恰恰是还没被公开的那些。

**而第一条规则高于这个机制**：不要反序列化不可信数据。如果那份数据可以用 JSON 这类有文档的格式表达，就用它 —— 这整类 bug 都来自平台"愿意重建任意对象"这件事，而一个**无法表达"类名"的格式，也就无法被说服去构造一个**。

### JSP 与表达式语言

**JSP 会编译成 Servlet。** `<% %>` 脚本片段就是 Java 源码，这也是为什么一旦 JSP 文件的内容受到影响，它就是一个代码执行面。

**表达式语言是一个求值器。** `${...}` 不是模板占位符 —— 它是**由容器求值的表达式**：

```jsp
${7*7}          <!-- 49 -->
${param.name}   <!-- 读一个请求参数 -->
${header.host}  <!-- 读一个请求头 -->
```

EL 2.2 及以后还允许对它够得着的对象做**方法调用**。所以一个把用户输入放进 EL 表达式的应用 —— 比如用某个参数拼一条消息模板 —— 等于交给攻击者一个求值器，它能读到请求数据，而且取决于具体实现与作用域里的对象，还能读到更多。

```java
// 那个错误的形状：用户输入变成了表达式的一部分
String expr = "${" + userInput + "}";
// 攻击者提交的不是一个名字，而是能读到他想要的东西的表达式
```

**输出转义是按标签决定的，不是按语言。** `<%= value %>` 原样写出；`<c:out value="${value}"/>` 会转义。决定权不在语言，在标签 —— 和 ASP.NET 那篇里"是否转义取决于所用的控件"是同一句话。

**包含有两种形式**，行为不同：

| 指令 | 何时被包含 |
|---|---|
| `<%@ include file="..." %>` | 翻译期 —— 静态，是被编译页面的一部分 |
| `<jsp:include page="..." />` | 请求期 —— 动态，是对另一个资源的一次独立请求 |

### Servlet 的路径函数，以及它们为什么互相不一致

这是整篇里对测试最有用的部分，因为"两个解析器"的模式在这里**在一个 API 内部**就看得见。

| 方法 | 返回什么 |
|---|---|
| `getRequestURI()` | 原始 URI 路径，**未解码**，含 context path |
| `getServletPath()` | 映射到该 Servlet 的部分，**已由容器解码** |
| `getPathInfo()` | Servlet 映射之后剩下的部分，已解码 |
| `getContextPath()` | 应用的前缀 |
| `getQueryString()` | 原始查询串，未解码 |

由此产生三个会在真实漏洞里出现的后果：

**一、检查一个、使用另一个。** 校验 `getRequestURI()` 的过滤器，和按 `getServletPath()` 路由的 Servlet，看的是不同的字符串，而差异里包括解码与规范化：

```
/app/admin/../user/list      -> getRequestURI 可能保留 ".."，getServletPath 可能已被规范化
/app/%2e%2e/admin            -> 解码决策因容器与配置而异
/app/list;jsessionid=ABC     -> 容器会剥掉路径参数；天真的检查可能不会
```

**二、路径参数。** Java 容器支持路径里的 `;name=value` 段，最常见的是 `;jsessionid=XYZ`。容器在映射之前把它们去掉；而读原始 URI 的安全检查看得见它们。这是一个绕过原语，而且它是语言与容器的约定，不是哪一方的 bug。

**三、编码的分隔符。** 路径里的 `%2f` 是否在映射之前被解码、从而是否变成分隔符，是**容器设置**（Tomcat 的编码斜杠处理是常用例子）。两个设置不同的组件，对"这条路径有几段"的意见不一致。

**推理方式**和本系列其他地方一样：列出会看到这个 URI 的组件（反向代理、连接器、容器映射器、过滤器、Servlet、框架路由），问每一个解码和规范化了什么。**漏洞就出现在某个决定由其中一个做出、却被另一个使用的地方。**

### Java 文件上传的语言层面事实

上传处理有自己的篇目；语言层面的事实是：

| API | 行为 |
|---|---|
| **Servlet 3.0 的 `Part`** | `part.getSubmittedFileName()` 返回**客户端提供的文件名**；更老的 `getName()` 在某些容器里返回的是完整路径，路径穿越就是从那来的 |
| **`commons-fileupload`** | `FileItem.getName()` 同样返回客户端名字，而该库自己的指引就是要把路径从里面剥掉 |
| **`MultipartFile`（Spring）** | `getOriginalFilename()` 是客户端名字；框架不做净化 |
| **`getContentType()`** | 在上面每一个 API 里，都是**客户端声明的**类型 |

这个模式和 PHP 那篇一致：**文件名与内容类型来自请求**，所以任何基于它们的检查，检查的都是攻击者提供的数据，而存储决策必须建立在服务器侧、规范化之后的值上。

### 识别这个技术栈

| 信号 | 它告诉你什么 |
|---|---|
| `JSESSIONID` cookie、URL 里的 `;jsessionid=` | Servlet 容器 |
| `Server: Apache-Coyote/1.1` | Tomcat 的连接器 |
| `X-Powered-By: Servlet/3.1 JSP/2.3` | Servlet 与 JSP 规范版本 |
| `.jsp`、`.do`、`.action`、`.jsf` | JSP、Struts 风格、JSF |
| 带包名的详细调用栈 | 框架与库的版本，以及一个丰富的类路径 |
| cookie、参数或请求体里的 `rO0AB` | 一段 base64 编码的 Java 序列化流 |

### 检测与缓解

- **不要反序列化不可信数据，并优先采用无法表达类名的格式。** 在确实需要 Java 序列化的地方，用**白名单**方式的 `ObjectInputFilter` 只放行应用需要的类，并把任何"已知 gadget 类黑名单"当作临时措施而不是控制。
- **对输入里的序列化签名与 EL 语法告警。** `rO0AB`（以及原始字节 `AC ED 00 05`）、`${`、`#{(`、`%24%7B` 出现在参数、cookie 或请求体里都是结构性异常：它们是**平台的词汇出现在了客户端的请求里**。这和 PHP 那篇里的 `php://`、NoSQL 那篇里 `$` 开头的键是同一个信号。
- **预期这是一个类路径问题，而不是应用问题。** gadget 不必是你的代码；它只需要在类路径上的某处。这就是为什么发现通常被报在那个反序列化入口上，而不是报在提供 gadget 的那个库上。
- **包装类型用 `equals` 比较，不要让安全决策落在 `==` 上。** 整数缓存的行为意味着这个 bug 在测试里看不见、在生产里存在 —— 这正是代码评审应当**刻意**去找的那类发现。
- **在比较边界之前先放大运算宽度，并且校验结果而不是输入。** `offset + length > size` 会溢出；`offset > size - length` 不会。
- **每一次字符串与字节之间的转换都显式传字符集。** 不带参数的 `getBytes()` 今天是可移植性 bug，而在两个组件对编码意见不一致时，就是与安全相关的。
- **把路径当成好几个字符串，并校验你将要据以行动的那个。** 读路由所用的同一个访问器、经过同样的规范化，并记住：路径参数与编码过的分隔符意味着，原始 URI 与映射后的路径可以指向不同的资源。
- **给正则设界，或者让它远离用户输入。** 引擎没有超时，所以一个无边界的模式配上攻击者可控的数据就是一个拒绝服务原语；修法是更简单的模式、对输入的长度限制，或者在匹配之前先做长度检查。
- **记日志时要记这几个访问器，而不只是"路径"。** 把 `getRequestURI`、`getServletPath`、`getPathInfo` 并排记进请求日志，能让一整类路由绕过变得可见 —— 因为那个不一致发生在其中两个值之间，而不在任何单独一个值内部。
