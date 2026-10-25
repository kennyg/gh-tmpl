/-!
# The template lookup order

This file models `discover.Find` and `discover.FindIn` in
`internal/discover/discover.go`.

A repository is a list of file paths. A source is what opening one
repository gives: an error, nothing, a missing repository, a repository whose
root cannot be read, or the files. The theorems say:

* `find_unreadable_first`: if the first source is missing or cannot be read,
  the lookup is an error. It never passes with no templates. This is the
  TMP-1 review finding F1.
* `find_first_with_templates`: a lookup with templates returns those of the
  first source that has any. Every source before it was skipped, missing as
  a fallback, or empty. The result keeps the problems of that source and of
  every repository before it, so a broken template in the first source is
  reported when a fallback supplies the templates. This is TMP-5.
* `find_no_templates_problems`: a lookup without templates keeps the
  problems of every source.
* `findIn_defaults`: the single-file template is the first candidate in the
  documented order: `.github/`, the root, `docs/`, and by name in each.
* `findIn_folder`: the folder templates come from the first template folder
  in that order that holds at least one template.

The model leaves out the file size limit and the limit of 100 templates per
folder. Every issue form in the model fails to parse, so a form is always a
problem.
-/

-- Some proofs share one simp set across many cases, and not every case
-- needs every lemma.
set_option linter.unusedSimpArgs false

namespace GhTmpl.Lookup

inductive Kind where
  | pr
  | issue
  deriving Repr, DecidableEq

/-- A path, one component per folder. -/
abbrev Path := List String

def Path.str (p : Path) : String := "/".intercalate p

def lower (s : String) : String := s.toLower

/-- `path.Ext`: the extension from the last dot, with the dot. -/
def splitExt (name : String) : String × String :=
  let cs := name.toList
  match (List.range cs.length).reverse.find? (fun i => cs[i]? == some '.') with
  | some i => (String.ofList (cs.take i), String.ofList (cs.drop i))
  | none => (name, "")

def base : Kind → String
  | .pr => "pull_request_template"
  | .issue => "issue_template"

def isSingle (k : Kind) (name : String) : Bool :=
  let (stem, ext) := splitExt name
  lower stem == base k && ["", ".md", ".markdown", ".txt"].contains (lower ext)

def isFolder (k : Kind) (name : String) : Bool := lower name == base k

/-- The entries directly in a folder: names, and whether each is a folder. -/
def children (fs : List Path) (dir : Path) : List (String × Bool) :=
  let es := fs.filterMap fun p =>
    if dir.isPrefixOf p then
      match p.drop dir.length with
      | [n] => some (n, false)
      | n :: _ :: _ => some (n, true)
      | [] => none
    else none
  let names := (es.map (·.1)).eraseDups
  let sorted := names.mergeSort (fun a b => !(b < a))
  sorted.map fun n => (n, es.any fun e => e.1 == n && e.2)

def hasDir (fs : List Path) (name : String) : Bool :=
  (children fs []).any fun e => e.1 == name && e.2

/-- The locations, in order of precedence. -/
def locations : List Path := [[".github"], [], ["docs"]]

/-- How a file in a template folder loads. -/
inductive Load where
  | template
  | problem
  | skip
  deriving Repr, DecidableEq

def loadInFolder (k : Kind) (name : String) : Load :=
  let (stem, ext) := splitExt name
  let ext := lower ext
  if [".md", ".markdown", ".txt"].contains ext then .template
  else if [".yml", ".yaml"].contains ext then
    if k != .issue || lower stem == "config" then .skip else .problem
  else .skip

/-- One entry in a location, in the order that discovery visits them. -/
structure Entry where
  path : Path
  isDir : Bool
  deriving Repr, DecidableEq

def entries (fs : List Path) : List Entry :=
  locations.flatMap fun loc =>
    if loc == [] || hasDir fs (loc.headD "") then
      (children fs loc).map fun (n, d) => ⟨loc ++ [n], d⟩
    else []

def Entry.name (e : Entry) : String := e.path.getLastD ""

/-- The templates and problems of one template folder. -/
def folderFiles (k : Kind) (fs : List Path) (dir : Path) : List Path × Nat :=
  let files := (children fs dir).filter (!·.2)
  (files.filterMap fun (n, _) => if loadInFolder k n == .template then some (dir ++ [n]) else none,
   (files.filter fun (n, _) => loadInFolder k n == .problem).length)

structure St where
  single : Bool := false
  folder : Bool := false
  /-- Template paths, and whether each is the single-file template. -/
  templates : List (Path × Bool) := []
  shadowed : List String := []
  problems : Nat := 0
  deriving Repr

/-- One step of discovery, with the tests of the entry already done. -/
def stepWith (sgl fld : Bool) (path : Path) (files : List Path × Nat) (st : St) : St :=
  if sgl then
    if st.single then { st with shadowed := st.shadowed ++ [path.str] }
    else { st with single := true, templates := st.templates ++ [(path, true)] }
  else if fld then
    if st.folder then { st with shadowed := st.shadowed ++ [path.str ++ "/"] }
    else
      { st with folder := files.1 != [], templates := st.templates ++ files.1.map (·, false),
                problems := st.problems + files.2 }
  else st

def isSingleEntry (k : Kind) (e : Entry) : Bool := !e.isDir && isSingle k e.name

def isFolderEntry (k : Kind) (e : Entry) : Bool := e.isDir && isFolder k e.name

def step (k : Kind) (fs : List Path) (st : St) (e : Entry) : St :=
  stepWith (isSingleEntry k e) (isFolderEntry k e) e.path (folderFiles k fs e.path) st

/-- The model of `FindIn` for a repository whose root can be read. -/
def findIn (k : Kind) (fs : List Path) : St :=
  (entries fs).foldl (step k fs) {}

/-- What opening and listing one source gives. -/
inductive Src where
  /-- The source function fails, for example with no credentials. -/
  | openErr
  /-- The source function returns no repository. -/
  | skip
  /-- The repository root does not exist, or the token cannot read it. -/
  | notFound
  /-- Listing the root fails for another reason. -/
  | readErr
  | repo (files : List Path)
  deriving Repr, DecidableEq

inductive Err where
  | openErr
  | notFound
  | readErr
  deriving Repr, DecidableEq

structure Result where
  /-- The index of the source that the result comes from. -/
  source : Option Nat
  templates : List (Path × Bool)
  shadowed : List String
  /-- The index of the source of each problem, in the order found. -/
  problems : List Nat
  deriving Repr

def findAux (k : Kind) : Nat → Result → List Src → Except Err Result
  | _, last, [] => .ok last
  | i, last, s :: ss =>
    match s with
    | .openErr => .error .openErr
    | .skip => findAux k (i + 1) last ss
    | .notFound => if i = 0 then .error .notFound else findAux k (i + 1) last ss
    | .readErr => .error .readErr
    | .repo fs =>
      let st := findIn k fs
      let problems := last.problems ++ List.replicate st.problems i
      if st.templates != [] then .ok ⟨some i, st.templates, st.shadowed, problems⟩
      else findAux k (i + 1) ⟨some i, [], st.shadowed, problems⟩ ss

/-- The model of `Find`. -/
def find (k : Kind) (srcs : List Src) : Except Err Result :=
  findAux k 0 ⟨none, [], [], []⟩ srcs

/-- The problems of source `i`, one source index for each problem. -/
def srcProblems (k : Kind) (i : Nat) : Src → List Nat
  | .repo fs => List.replicate (findIn k fs).problems i
  | _ => []

/-- The problems of the sources `ss`, the first of which has index `i`. -/
def problemsFrom (k : Kind) : Nat → List Src → List Nat
  | _, [] => []
  | i, s :: ss => srcProblems k i s ++ problemsFrom k (i + 1) ss

/-! ## Theorems about the sources -/

/-- F1: a first source that is missing or cannot be read is an error. -/
theorem find_unreadable_first (k : Kind) (s : Src) (ss : List Src)
    (h : s = .openErr ∨ s = .notFound ∨ s = .readErr) : ∃ e, find k (s :: ss) = .error e := by
  rcases h with rfl | rfl | rfl <;> simp [find, findAux]

/-- A source that the lookup may pass over: skipped, a missing fallback, or a
repository with no templates. -/
def passable (k : Kind) (i : Nat) : Src → Prop
  | .skip => True
  | .notFound => i ≠ 0
  | .repo fs => (findIn k fs).templates = []
  | _ => False

theorem findAux_first (k : Kind) (i : Nat) (last : Result) (ss : List Src) (r : Result)
    (hl : last.templates = []) (h : findAux k i last ss = .ok r) (hr : r.templates ≠ []) :
    ∃ pre fs post, ss = pre ++ .repo fs :: post ∧ r.source = some (i + pre.length) ∧
      (findIn k fs).templates = r.templates ∧
      (∀ j (hj : j < pre.length), passable k (i + j) pre[j]) ∧
      r.problems = last.problems ++ problemsFrom k i (pre ++ [.repo fs]) := by
  induction ss generalizing i last with
  | nil =>
    simp only [findAux, Except.ok.injEq] at h
    subst h
    exact absurd hl hr
  | cons s ss ih =>
    have shift : ∀ pre fs post, ss = pre ++ .repo fs :: post → r.source = some (i + 1 + pre.length) →
        (findIn k fs).templates = r.templates → (∀ j (hj : j < pre.length), passable k (i + 1 + j) pre[j]) →
        r.problems = last.problems ++ srcProblems k i s ++ problemsFrom k (i + 1) (pre ++ [.repo fs]) →
        passable k i s →
        ∃ pre fs post, s :: ss = pre ++ .repo fs :: post ∧ r.source = some (i + pre.length) ∧
          (findIn k fs).templates = r.templates ∧
          (∀ j (hj : j < pre.length), passable k (i + j) pre[j]) ∧
          r.problems = last.problems ++ problemsFrom k i (pre ++ [.repo fs]) := by
      intro pre fs post hss hsrc htp hpass hprob hs
      refine ⟨s :: pre, fs, post, by simp [hss], by simp [hsrc]; omega, htp, ?_, ?_⟩
      · intro j hj
        cases j with
        | zero => simpa using hs
        | succ j =>
          have := hpass j (by simp at hj; omega)
          simp only [List.getElem_cons_succ]
          rwa [show i + (j + 1) = i + 1 + j by omega]
      · simp [hprob, problemsFrom, List.append_assoc]
    cases s with
    | openErr => simp [findAux] at h
    | readErr => simp [findAux] at h
    | skip =>
      simp only [findAux] at h
      obtain ⟨pre, fs, post, a, b, c, d, e⟩ := ih (i + 1) last hl h
      exact shift pre fs post a b c d (by simp [e, srcProblems]) trivial
    | notFound =>
      simp only [findAux] at h
      split at h
      · simp at h
      · rename_i hi
        obtain ⟨pre, fs, post, a, b, c, d, e⟩ := ih (i + 1) last hl h
        exact shift pre fs post a b c d (by simp [e, srcProblems]) hi
    | repo fs =>
      simp only [findAux] at h
      split at h
      · rename_i hne
        simp only [Except.ok.injEq] at h
        subst h
        exact ⟨[], fs, ss, rfl, by simp, rfl, fun j hj => by simp at hj,
          by simp [problemsFrom, srcProblems]⟩
      · rename_i he
        simp only [bne_iff_ne, ne_eq, Decidable.not_not] at he
        obtain ⟨pre, fs', post, a, b, c, d, e⟩ := ih (i + 1) _ rfl h
        exact shift pre fs' post a b c d (by simp [e, srcProblems, List.append_assoc]) he

/-- A lookup with templates returns those of the first source that has any.
It keeps the problems of that source and of every source before it. -/
theorem find_first_with_templates (k : Kind) (ss : List Src) (r : Result)
    (h : find k ss = .ok r) (hr : r.templates ≠ []) :
    ∃ pre fs post, ss = pre ++ .repo fs :: post ∧ r.source = some pre.length ∧
      (findIn k fs).templates = r.templates ∧
      (∀ j (hj : j < pre.length), passable k j pre[j]) ∧
      r.problems = problemsFrom k 0 (pre ++ [.repo fs]) := by
  obtain ⟨pre, fs, post, a, b, c, d, e⟩ := findAux_first k 0 _ ss r rfl h hr
  exact ⟨pre, fs, post, a, by simpa using b, c, fun j hj => by simpa using d j hj, by simpa using e⟩

theorem findAux_no_templates (k : Kind) (i : Nat) (last : Result) (ss : List Src) (r : Result)
    (h : findAux k i last ss = .ok r) (hr : r.templates = []) :
    r.problems = last.problems ++ problemsFrom k i ss := by
  induction ss generalizing i last with
  | nil =>
    simp only [findAux, Except.ok.injEq] at h
    subst h
    simp [problemsFrom]
  | cons s ss ih =>
    cases s with
    | openErr => simp [findAux] at h
    | readErr => simp [findAux] at h
    | skip =>
      simp only [findAux] at h
      simp [ih _ _ h, problemsFrom, srcProblems]
    | notFound =>
      simp only [findAux] at h
      split at h
      · simp at h
      · simp [ih _ _ h, problemsFrom, srcProblems]
    | repo fs =>
      simp only [findAux] at h
      split at h
      · rename_i hne
        simp only [Except.ok.injEq] at h
        subst h
        simp_all
      · simp [ih _ _ h, problemsFrom, srcProblems, List.append_assoc]

/-- A lookup without templates keeps the problems of every source. -/
theorem find_no_templates_problems (k : Kind) (ss : List Src) (r : Result)
    (h : find k ss = .ok r) (hr : r.templates = []) :
    r.problems = problemsFrom k 0 ss := by
  simpa using findAux_no_templates k 0 _ ss r h hr

/-! ## Theorems about one repository -/

/-- The single-file templates found. -/
def defaults (st : St) : List Path := (st.templates.filter (·.2)).map (·.1)

/-- The folder templates found. -/
def folderTemplates (st : St) : List Path := (st.templates.filter (!·.2)).map (·.1)

/-- The single-file candidates, in the order that discovery visits them. -/
def singleCandidates (k : Kind) (es : List Entry) : List Path :=
  (es.filter (isSingleEntry k)).map (·.path)

/-- The templates of each template folder, in the order that discovery
visits them. -/
def folderCandidates (k : Kind) (fs : List Path) (es : List Entry) : List (List Path) :=
  (es.filter (isFolderEntry k)).map fun e => (folderFiles k fs e.path).1

/-- The first list that is not empty. -/
def firstNonempty {α : Type} : List (List α) → List α
  | [] => []
  | x :: xs => if x.isEmpty then firstNonempty xs else x

theorem pairs_false (l : List Path) : ((l.map (·, false)).filter (!·.2)).map (·.1) = l := by
  induction l <;> simp_all

theorem step_defaults (k : Kind) (fs : List Path) (st : St) (e : Entry) :
    defaults (step k fs st e) =
      if isSingleEntry k e && !st.single then defaults st ++ [e.path] else defaults st := by
  have hx : isSingleEntry k e = true → isFolderEntry k e = false := by
    simp only [isSingleEntry, isFolderEntry]; cases e.isDir <;> simp
  unfold step
  revert hx
  generalize isSingleEntry k e = sgl
  generalize isFolderEntry k e = fld
  obtain ⟨single, folder, templates, shadowed, problems⟩ := st
  cases sgl <;> cases fld <;> cases single <;> cases folder <;>
  simp [stepWith, defaults, folderTemplates, List.filter_append, Function.comp_def, pairs_false]

theorem step_single (k : Kind) (fs : List Path) (st : St) (e : Entry) :
    (step k fs st e).single = (st.single || isSingleEntry k e) := by
  have hx : isSingleEntry k e = true → isFolderEntry k e = false := by
    simp only [isSingleEntry, isFolderEntry]; cases e.isDir <;> simp
  unfold step
  revert hx
  generalize isSingleEntry k e = sgl
  generalize isFolderEntry k e = fld
  obtain ⟨single, folder, templates, shadowed, problems⟩ := st
  cases sgl <;> cases fld <;> cases single <;> cases folder <;>
  simp [stepWith, defaults, folderTemplates, List.filter_append, Function.comp_def, pairs_false]

theorem step_folderTemplates (k : Kind) (fs : List Path) (st : St) (e : Entry) :
    folderTemplates (step k fs st e) =
      if isFolderEntry k e && !st.folder then folderTemplates st ++ (folderFiles k fs e.path).1
      else folderTemplates st := by
  have hx : isSingleEntry k e = true → isFolderEntry k e = false := by
    simp only [isSingleEntry, isFolderEntry]; cases e.isDir <;> simp
  unfold step
  revert hx
  generalize isSingleEntry k e = sgl
  generalize isFolderEntry k e = fld
  obtain ⟨single, folder, templates, shadowed, problems⟩ := st
  cases sgl <;> cases fld <;> cases single <;> cases folder <;>
  simp [stepWith, defaults, folderTemplates, List.filter_append, Function.comp_def, pairs_false]

theorem step_folder (k : Kind) (fs : List Path) (st : St) (e : Entry) :
    (step k fs st e).folder =
      if isFolderEntry k e && !st.folder then (folderFiles k fs e.path).1 != [] else st.folder := by
  have hx : isSingleEntry k e = true → isFolderEntry k e = false := by
    simp only [isSingleEntry, isFolderEntry]; cases e.isDir <;> simp
  unfold step
  revert hx
  generalize isSingleEntry k e = sgl
  generalize isFolderEntry k e = fld
  obtain ⟨single, folder, templates, shadowed, problems⟩ := st
  cases sgl <;> cases fld <;> cases single <;> cases folder <;>
  simp [stepWith, defaults, folderTemplates, List.filter_append, Function.comp_def, pairs_false]

theorem run_defaults (k : Kind) (fs : List Path) (es : List Entry) (st : St) :
    defaults (es.foldl (step k fs) st) =
      if st.single then defaults st else defaults st ++ (singleCandidates k es).take 1 := by
  induction es generalizing st with
  | nil => simp [singleCandidates]
  | cons e es ih =>
    simp only [List.foldl_cons, ih, step_single, step_defaults, singleCandidates, List.filter_cons]
    cases hs : st.single <;> cases he : isSingleEntry k e <;> simp [singleCandidates]

theorem run_folderTemplates (k : Kind) (fs : List Path) (es : List Entry) (st : St) :
    folderTemplates (es.foldl (step k fs) st) =
      if st.folder then folderTemplates st
      else folderTemplates st ++ firstNonempty (folderCandidates k fs es) := by
  induction es generalizing st with
  | nil => simp [folderCandidates, firstNonempty]
  | cons e es ih =>
    simp only [List.foldl_cons, ih, step_folder, step_folderTemplates]
    cases hf : st.folder <;> cases he : isFolderEntry k e <;>
      cases h : (folderFiles k fs e.path).1 <;>
      simp [folderCandidates, List.filter_cons, he, h, firstNonempty]

/-- The single-file template is the first candidate in the documented order.
Later candidates are shadowed. -/
theorem findIn_defaults (k : Kind) (fs : List Path) :
    defaults (findIn k fs) = (singleCandidates k (entries fs)).take 1 := by
  rw [findIn, run_defaults]; simp [defaults]

/-- The folder templates come from the first template folder that holds at
least one template. -/
theorem findIn_folder (k : Kind) (fs : List Path) :
    folderTemplates (findIn k fs) = firstNonempty (folderCandidates k fs (entries fs)) := by
  rw [findIn, run_folderTemplates]; simp [folderTemplates]

end GhTmpl.Lookup
