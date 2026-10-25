import GhTmpl

/-!
# The differential test corpus

`lake exe corpus <repo>` runs the models on generated cases and writes the
inputs and the results as JSON lines. The Go tests read them and check that
the Go functions give the same results:

* `internal/cli/testdata/lean/argv.jsonl`: `splitCreate` and `scanCreate`.
* `internal/check/testdata/lean/check.jsonl`: `check.Markdown`.
* `internal/discover/testdata/lean/find.jsonl`: `discover.Find`.

The cases come from a fixed seed, so the files change only when a model or
the generator changes.
-/

open GhTmpl

/-! ## JSON -/

def hexDigit (n : Nat) : Char :=
  if n < 10 then Char.ofNat (48 + n) else Char.ofNat (87 + n)

def jsonStr (s : String) : String :=
  let body := s.toList.flatMap fun c =>
    if c == '"' then ['\\', '"']
    else if c == '\\' then ['\\', '\\']
    else if c.toNat < 0x20 then
      ['\\', 'u', '0', '0', hexDigit (c.toNat / 16), hexDigit (c.toNat % 16)]
    else [c]
  "\"" ++ String.ofList body ++ "\""

def jsonArr (xs : List String) : String := "[" ++ ",".intercalate xs ++ "]"

def jsonObj (kvs : List (String × String)) : String :=
  "{" ++ ",".intercalate (kvs.map fun (k, v) => jsonStr k ++ ":" ++ v) ++ "}"

def jsonOpt (o : Option String) : String :=
  match o with
  | some s => jsonStr s
  | none => "null"

def jsonBool (b : Bool) : String := if b then "true" else "false"

/-! ## A fixed pseudo-random sequence -/

structure Rng where
  state : Nat

def Rng.next (r : Rng) : Nat × Rng :=
  let s := (r.state * 6364136223846793005 + 1442695040888963407) % 18446744073709551616
  (s / 65536, ⟨s⟩)

def Rng.below (r : Rng) (n : Nat) : Nat × Rng :=
  let (x, r) := r.next
  (x % n, r)

def Rng.pick {α : Type} [Inhabited α] (r : Rng) (xs : List α) : α × Rng :=
  let (i, r) := r.below xs.length
  (xs[i]!, r)

/-- Draw `n` values with `f`. -/
def Rng.many {α : Type} (r : Rng) (n : Nat) (f : Rng → α × Rng) : List α × Rng :=
  (List.range n).foldl (fun (acc, r) _ => let (x, r) := f r; (acc ++ [x], r)) ([], r)

/-! ## Argument cases -/

def ownLongs : List (String × Bool) :=
  [("tmpl-name", true), ("tmpl-strict", false), ("tmpl-allow-unchecked", false),
   ("tmpl-offline", false), ("tmpl-skip", false), ("help", false)]

/-- `ghFlags` in `internal/cli/create.go`. -/
def ghTable : String → List (String × Option Char)
  | "pr" =>
    [("assignee", some 'a'), ("attach", none), ("base", some 'B'), ("body", some 'b'),
     ("body-file", some 'F'), ("head", some 'H'), ("label", some 'l'), ("milestone", some 'm'),
     ("project", some 'p'), ("recover", none), ("reviewer", some 'r'), ("template", some 'T'),
     ("title", some 't'), ("repo", some 'R')]
  | _ =>
    [("assignee", some 'a'), ("attach", none), ("blocked-by", none), ("blocking", none),
     ("body", some 'b'), ("body-file", some 'F'), ("label", some 'l'), ("milestone", some 'm'),
     ("parent", none), ("project", some 'p'), ("recover", none), ("template", some 'T'),
     ("title", some 't'), ("repo", some 'R'), ("type", none)]

def table (kind : String) : Argv.Table := ⟨ownLongs, ghTable kind⟩

def words : List String :=
  ["--title", "-t", "x", "--tmpl-skip", "--tmpl-name", "feat", "--tmpl-name=a", "--body",
   "-b", "-F", "--body-file=b.md", "-dF", "-Fb.md", "-db", "--", "-h", "--draft", "--repo",
   "-R=o/r", "--tmpl-x", "-", "", "--type", "--parent", "-b=", "--body-file", "--tmpl-strict=1",
   "--tmpl-skip=false", "--tmpl-skip=0", "--tmpl-offline=true", "--tmpl-skip=yes",
   "--tmpl-allow-unchecked="]

/-- The switches of `pr create` and `issue create`. -/
def switches : List String :=
  ownLongs.filterMap fun (n, v) => if !v && n != "help" then some n else none

def argvCase (kind : String) (args : List String) : String :=
  let t := table kind
  let (own, gh) := Argv.split t args
  let scan := match Argv.scan t gh {} with
    | some s => jsonObj [("body", jsonOpt s.body), ("bodyFile", jsonOpt s.bodyFile),
                         ("repo", jsonOpt s.repo)]
    | none => "null"
  jsonObj [("kind", jsonStr kind), ("args", jsonArr (args.map jsonStr)),
           ("own", jsonArr (own.map jsonStr)), ("gh", jsonArr (gh.map jsonStr)), ("scan", scan),
           ("parses", jsonBool (Argv.parses t args)), ("tmplName", jsonOpt (Argv.tmplName t args)),
           ("switches", jsonObj (switches.map fun n => (n, jsonBool (Argv.switchOn t n args))))]

def tableLine (kind : String) : String :=
  jsonObj [("kind", jsonStr kind),
           ("ownFlags", jsonObj (ownLongs.map fun (n, v) => (n, jsonBool v))),
           ("ghFlags", jsonObj ((ghTable kind).map fun (n, s) =>
             (n, jsonStr (match s with | some c => String.singleton c | none => ""))))]

def argvLines : List String := Id.run do
  let mut out := ["pr", "issue"].map tableLine
  let mut r : Rng := ⟨1⟩
  for kind in ["pr", "issue"] do
    out := out ++ [argvCase kind []]
    for a in words do
      out := out ++ [argvCase kind [a]]
      for b in words do
        out := out ++ [argvCase kind [a, b]]
    for _ in List.range 400 do
      let (n, r1) := r.below 6
      let (args, r2) := r1.many (n + 3) (·.pick words)
      r := r2
      out := out ++ [argvCase kind args]
  return out

/-! ## Check cases -/

open Check in
def renderLines (d : List Line) : String := jsonArr (d.map fun l => jsonStr l.render)

open Check in
def templatePool : List Line :=
  [.heading "Summary", .heading "Testing", .heading "Notes", .text "Describe the change.",
   .text "Example text", .comment "Explain why.", .comment "markdownlint-disable",
   .comment "List the steps.", .box false "Tests added", .box false "Docs updated",
   .box false "Bug fix", .box true "Tests added", .blank]

open Check in
instance : Inhabited Line := ⟨.blank⟩

open Check in
/-- A change to one template line in the body. -/
def mutate (r : Rng) (l : Line) : List Line × Rng :=
  let (c, r) := r.below 10
  match l with
  | .heading t =>
    if c < 6 then ([l], r)
    else if c < 8 then ([.heading t.toUpper], r)
    else if c < 9 then ([], r)
    else ([.heading "Extra"], r)
  | .text _ =>
    if c < 4 then ([l], r)
    else if c < 7 then ([.text "My own text."], r)
    else if c < 8 then ([.text "N/A"], r)
    else ([], r)
  | .comment _ =>
    if c < 4 then ([l], r)
    else if c < 8 then ([], r)
    else ([.comment "mine"], r)
  | .box b label =>
    if c < 4 then ([l], r)
    else if c < 7 then ([.box (!b) label], r)
    else if c < 9 then ([], r)
    else ([.box true label.toLower], r)
  | .blank => ([l], r)

open Check in
def extraPool : List Line :=
  [.heading "Extra", .text "Added text.", .comment "mine", .box false "Tests added",
   .box true "Docs updated", .blank, .heading "testing"]

open Check in
def genBody (r : Rng) (t : List Line) : List Line × Rng := Id.run do
  let mut r := r
  let (mode, r1) := r.below 20
  r := r1
  if mode == 0 then return ([], r)
  if mode == 1 then return ([.blank, .blank], r)
  if mode == 2 then
    let (n, r2) := r.below 6
    return r2.many n (·.pick extraPool)
  let mut out := []
  for l in t do
    let (ls, r2) := mutate r l
    r := r2
    out := out ++ ls
    let (x, r3) := r.below 8
    r := r3
    if x == 0 then
      let (e, r4) := r.pick extraPool
      r := r4
      out := out ++ [e]
  return (out, r)

open Check in
def checkCase (t b : List Line) (o : Opts) : String :=
  let fs := check t b o
  jsonObj [("template", renderLines t), ("body", renderLines b),
           ("strict", jsonBool o.strict), ("allowUnchecked", jsonBool o.allowUnchecked),
           ("findings", jsonArr (fs.map fun f => jsonObj [
             ("severity", jsonStr (match f.severity with | .error => "error" | .warning => "warning")),
             ("code", jsonStr f.code), ("section", jsonStr f.sec), ("line", toString f.line)]))]

open Check in
def checkLines : List String := Id.run do
  let mut out := []
  let mut r : Rng := ⟨2⟩
  for _ in List.range 1200 do
    let (n, r1) := r.below 9
    let (t, r2) := r1.many (n + 1) (·.pick templatePool)
    let (b, r3) := genBody r2 t
    let (oi, r4) := r3.below 4
    r := r4
    out := out ++ [checkCase t b ⟨oi % 2 == 1, oi / 2 == 1⟩]
  return out

/-! ## Lookup cases -/

open Lookup

def filePool : List Path :=
  [[".github", "pull_request_template.md"], ["pull_request_template.md"],
   ["docs", "pull_request_template.md"], [".github", "PULL_REQUEST_TEMPLATE.txt"],
   ["docs", "PULL_REQUEST_TEMPLATE"], [".github", "pull_request_template", "a.md"],
   [".github", "PULL_REQUEST_TEMPLATE", "b.md"], [".github", "PULL_REQUEST_TEMPLATE", "notes.json"],
   [".github", "PULL_REQUEST_TEMPLATE", "form.yml"], ["docs", "pull_request_template", "c.md"],
   ["pull_request_template", "d.md"], [".github", "pull_request_template", "sub", "x.md"],
   [".github", "issue_template.md"], [".github", "ISSUE_TEMPLATE", "bug.md"],
   [".github", "ISSUE_TEMPLATE", "config.yml"], [".github", "ISSUE_TEMPLATE", "broken.yml"],
   ["docs", "ISSUE_TEMPLATE", "feature.md"], ["ISSUE_TEMPLATE.md"],
   ["docs", "issue_template.markdown"], ["DOCS", "pull_request_template.md"], ["README.md"],
   ["ISSUE_TEMPLATE", "form.yaml"], ["ISSUE_TEMPLATE", "Config.YML"],
   ["docs", "pull_request_template.md.bak"]]

def genRepo (r : Rng) : List Path × Rng :=
  filePool.foldl (fun (acc, r) p =>
    let (x, r) := r.below 4
    (if x == 0 then acc ++ [p] else acc, r)) ([], r)

def genSrc (r : Rng) : Src × Rng :=
  let (c, r) := r.below 12
  if c == 0 then (.openErr, r)
  else if c == 1 then (.skip, r)
  else if c == 2 then (.notFound, r)
  else if c == 3 then (.readErr, r)
  else if c == 4 then (.repo [], r)
  else let (fs, r) := genRepo r; (.repo fs, r)

def srcJson : Src → String
  | .openErr => jsonObj [("type", jsonStr "openErr")]
  | .skip => jsonObj [("type", jsonStr "skip")]
  | .notFound => jsonObj [("type", jsonStr "notFound")]
  | .readErr => jsonObj [("type", jsonStr "readErr")]
  | .repo fs => jsonObj [("type", jsonStr "repo"), ("files", jsonArr (fs.map (jsonStr ·.str)))]

def findCase (kind : Kind) (srcs : List Src) : String :=
  let res := match find kind srcs with
    | .error e => jsonObj [("error", jsonStr (match e with
        | .openErr => "openErr" | .notFound => "notFound" | .readErr => "readErr"))]
    | .ok r => jsonObj [
        ("source", match r.source with | some i => toString i | none => "null"),
        ("templates", jsonArr (r.templates.map fun (p, d) =>
          jsonObj [("path", jsonStr p.str), ("default", jsonBool d)])),
        ("shadowed", jsonArr (r.shadowed.map jsonStr)),
        ("problems", toString r.problems)]
  jsonObj [("kind", jsonStr (match kind with | .pr => "pr" | .issue => "issue")),
           ("sources", jsonArr (srcs.map srcJson)), ("result", res)]

def findLines : List String := Id.run do
  let mut out := []
  let mut r : Rng := ⟨3⟩
  for _ in List.range 800 do
    let (n, r1) := r.below 3
    let (srcs, r2) := r1.many (n + 1) genSrc
    let (k, r3) := r2.below 2
    r := r3
    out := out ++ [findCase (if k == 0 then .pr else .issue) srcs]
  return out

/-! ## Main -/

def write (root : System.FilePath) (rel : String) (lines : List String) : IO Unit := do
  let p := root / rel
  if let some dir := p.parent then IO.FS.createDirAll dir
  IO.FS.writeFile p (String.join (lines.map (· ++ "\n")))
  IO.println s!"wrote {lines.length} cases to {rel}"

def main (args : List String) : IO UInt32 := do
  let root : System.FilePath := args.headD ".."
  write root "internal/cli/testdata/lean/argv.jsonl" argvLines
  write root "internal/check/testdata/lean/check.jsonl" checkLines
  write root "internal/discover/testdata/lean/find.jsonl" findLines
  return 0
