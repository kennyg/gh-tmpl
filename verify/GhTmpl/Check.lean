/-!
# The pass and fail rules of the Markdown check

This file models `check.Markdown` in `internal/check/check.go`.

The model reads a document that is already split into lines of five kinds:
headings, text, comments, checkboxes and blank lines. `Line.render` writes
each kind the way the Go scanner reads it, one line per kind. The Go
scanner, with its regular expressions, code fences and comments over several
lines, is not in the model. The differential test feeds rendered documents
to the Go code, so it covers the scanner only for these simple lines.

The theorems are about a body that is not blank. They say:

* `pass_keeps_headings`: if the check passes, the body has each template
  heading at least as often as the template has it.
* `missing_heading_count`: the check reports exactly as many
  `missing-heading` findings for a heading as the body lacks.
* `pass_has_no_placeholder` and `placeholder_reported`: a pass means no
  template comment is left, and each left comment is reported at its line.
* `pass_touches_checklists`: if the check passes without
  `--allow-unchecked`, each template checklist has a touched item.
* `strict_pass_touches_every_checkbox`: with `--strict`, a pass means every
  template checkbox is touched.
* `untouched_reported`: each untouched checkbox is in a finding.

The rules for empty and unchanged sections are in the model and in the
differential test, but they have no theorems.
-/

-- Some proofs share one simp set across many cases, and not every case
-- needs every lemma.
set_option linter.unusedSimpArgs false

namespace GhTmpl.Check

/-- One line of a document. -/
inductive Line where
  | heading (text : String)
  | text (s : String)
  | comment (c : String)
  | box (checked : Bool) (label : String)
  | blank
  deriving Repr, DecidableEq

/-- The Markdown for a line, as the Go scanner reads it. -/
def Line.render : Line → String
  | .heading t => "## " ++ t
  | .text s => s
  | .comment c => "<!-- " ++ c ++ " -->"
  | .box b l => (if b then "- [x] " else "- [ ] ") ++ l
  | .blank => ""

inductive Severity where
  | error
  | warning
  deriving Repr, DecidableEq

/-- A finding. `subject` is for the theorems only: the heading key, the
comment or the checkbox key that the finding is about. -/
structure Finding where
  severity : Severity
  code : String
  sec : String
  line : Nat
  subject : String
  deriving Repr, DecidableEq

structure Opts where
  strict : Bool := false
  allowUnchecked : Bool := false
  deriving Repr

/-- Heading keys ignore case. The model has no emphasis markers or colons. -/
def key (s : String) : String := s.toLower

def label (s : String) : String := "## " ++ s

/-- The lines with their 0-based index. -/
def indexed (d : List Line) : List (Nat × Line) :=
  d.zipIdx.map fun (l, i) => (i, l)

def headings (d : List Line) : List (Nat × String) :=
  (indexed d).filterMap fun
    | (i, .heading t) => some (i, t)
    | _ => none

/-- The template headings that can match: those with a key. -/
def usable (d : List Line) : List (Nat × String) :=
  (headings d).filter fun (_, t) => key t != ""

/-! ## Greedy matching -/

/-- Take the first element that satisfies `p`, and return the rest. -/
def takeFirst {α : Type} (p : α → Bool) : List α → Option (α × List α)
  | [] => none
  | x :: xs =>
    if p x then some (x, xs)
    else (takeFirst p xs).map fun (y, ys) => (y, x :: ys)

theorem takeFirst_none {α : Type} (p : α → Bool) (l : List α)
    (h : takeFirst p l = none) : l.countP p = 0 := by
  induction l with
  | nil => rfl
  | cons x xs ih =>
    unfold takeFirst at h
    by_cases hx : p x
    · simp [hx] at h
    · simp only [hx] at h
      simp [List.countP_cons, hx, ih (by simpa using h)]

theorem takeFirst_some {α : Type} (p : α → Bool) (l : List α) (y : α) (l' : List α)
    (h : takeFirst p l = some (y, l')) :
    p y = true ∧ ∀ q : α → Bool, l.countP q = l'.countP q + (if q y then 1 else 0) := by
  induction l generalizing l' with
  | nil => simp [takeFirst] at h
  | cons x xs ih =>
    unfold takeFirst at h
    by_cases hx : p x
    · simp only [hx, ite_true, Option.some.injEq, Prod.mk.injEq] at h
      obtain ⟨rfl, rfl⟩ := h
      refine ⟨hx, fun q => ?_⟩
      simp [List.countP_cons]
    · simp only [hx] at h
      cases ht : takeFirst p xs with
      | none => simp [ht] at h
      | some r =>
        obtain ⟨y', ys⟩ := r
        simp only [ht, Option.map_some, Option.some.injEq, Prod.mk.injEq] at h
        obtain ⟨rfl, rfl⟩ := h
        obtain ⟨hp, hc⟩ := ih ys ht
        refine ⟨hp, fun q => ?_⟩
        simp only [List.countP_cons, hc q]
        omega

/-- `matchHeadings` in Go: each template key takes the first unused body
heading with the same key. The result is the body line, or `none`. -/
def matchH : List (Nat × String) → List String → List (Option Nat)
  | _, [] => []
  | rem, k :: ks =>
    match takeFirst (fun p => p.2 == k) rem with
    | some (p, rem') => some p.1 :: matchH rem' ks
    | none => none :: matchH rem ks

/-- How many times the key `k` is left without a match. -/
def unmatched (k : String) (ks : List String) (m : List (Option Nat)) : Nat :=
  ((ks.zip m).filter fun p => p.1 == k && p.2.isNone).length

theorem matchH_length (rem : List (Nat × String)) (ks : List String) :
    (matchH rem ks).length = ks.length := by
  induction ks generalizing rem with
  | nil => rfl
  | cons k ks ih =>
    unfold matchH
    split <;> simp [ih]

/-- The heart of `missing-heading`: greedy matching leaves a key without a
match exactly as often as the template has it more often than the body. -/
theorem matchH_unmatched (rem : List (Nat × String)) (ks : List String) (k : String) :
    unmatched k ks (matchH rem ks) = ks.count k - rem.countP (fun p => p.2 == k) := by
  induction ks generalizing rem with
  | nil => simp [unmatched, matchH]
  | cons k' ks ih =>
    unfold matchH
    split
    · rename_i p rem' h
      obtain ⟨hp, hc⟩ := takeFirst_some _ _ _ _ h
      have := ih rem'
      simp only [unmatched, List.zip_cons_cons, List.filter_cons, Option.isNone_some,
        Bool.and_false] at this ⊢
      simp only [Bool.false_eq_true, ite_false]
      rw [this, hc (fun p => p.2 == k)]
      simp only [beq_iff_eq] at hp
      by_cases hk : k' = k
      · subst hk; simp [hp, List.count_cons]
      · have : ¬ (p.2 = k) := by rw [hp]; exact hk
        simp [List.count_cons, this, hk]
    · rename_i h
      have h0 := takeFirst_none _ _ h
      have := ih rem
      simp only [unmatched, List.zip_cons_cons, List.filter_cons, Option.isNone_none,
        Bool.and_true] at this ⊢
      by_cases hk : k' = k
      · subst hk
        have h0' : rem.countP (fun p => p.2 == k') = 0 := h0
        simp [this, h0', List.count_cons]
      · simp [hk, this, List.count_cons]

/-! ## The rules -/

/-- `doc.section`: a section ends at the next line in `ends` after `start`,
or at the end of the document. -/
def sectionEnd (n : Nat) (ends : List Nat) (start : Nat) : Nat :=
  ends.foldl (fun e x => if start < x && x < e then x else e) n

def inRange (lo hi : Nat) (p : Nat × Line) : Bool := lo ≤ p.1 && p.1 < hi

/-- `doc.content`: the visible text of the lines, without blank lines. -/
def content (d : List Line) (lo hi : Nat) : List String :=
  ((indexed d).filter (inRange lo hi)).filterMap fun
    | (_, .comment _) | (_, .blank) => none
    | (_, l) => some l.render

def hasComments (d : List Line) (lo hi : Nat) : Bool :=
  ((indexed d).filter (inRange lo hi)).any fun
    | (_, .comment c) => c != ""
    | _ => false

/-- Text that is not a checkbox. -/
def hasProse (d : List Line) (lo hi : Nat) : Bool :=
  ((indexed d).filter (inRange lo hi)).any fun
    | (_, .text s) => s != ""
    | (_, .heading _) => true
    | _ => false

/-- The findings for one template heading `(tl, tx)` and its match. -/
def headingFinding (t b : List Line) (tEnds bEnds : List Nat) :
    (Nat × String) × Option Nat → List Finding
  | ((_, tx), none) => [⟨.error, "missing-heading", label tx, 0, key tx⟩]
  | ((tl, tx), some bl) =>
    let tTo := sectionEnd t.length tEnds tl
    let tc := content t (tl + 1) tTo
    if tc.isEmpty && !hasComments t (tl + 1) tTo then []
    else
      let bc := content b (bl + 1) (sectionEnd b.length bEnds bl)
      if bc.isEmpty then [⟨.error, "empty-section", label tx, bl + 1, key tx⟩]
      else if hasProse t (tl + 1) tTo && tc == bc then
        [⟨.error, "unchanged-section", label tx, bl + 1, key tx⟩]
      else []

def headingPart (t b : List Line) : List Finding :=
  let th := usable t
  let m := matchH ((headings b).map fun p => (p.1, key p.2)) (th.map fun p => key p.2)
  (th.zip m).flatMap (headingFinding t b (th.map (·.1)) (m.filterMap id))

/-- Comments that configure tools may stay. -/
def directives : List String :=
  ["markdownlint", "prettier-ignore", "cspell", "vale", "textlint", "toc",
   "all-contributors", "lint", "eslint", "remark"]

def isDirective (c : String) : Bool := directives.any (c.toLower.startsWith ·)

def placeholders (t : List Line) : List String :=
  t.filterMap fun
    | .comment c => if c != "" && !isDirective c then some c else none
    | _ => none

/-- The label of the last heading at or above line `i`. -/
def sectionAt (hs : List (Nat × String)) (i : Nat) : String :=
  hs.foldl (fun acc (l, x) => if l ≤ i then label x else acc) ""

def commentPart (t b : List Line) : List Finding :=
  let ps := placeholders t
  (indexed b).filterMap fun
    | (i, .comment c) =>
      if ps.contains c then some ⟨.error, "placeholder-comment", sectionAt (headings b) i, i + 1, c⟩
      else none
    | _ => none

/-- A template checkbox: its line, its section label and its key. -/
structure TBox where
  line : Nat
  sec : String
  key : String

/-- A body checkbox: its line, whether it is checked, and its key. -/
structure BBox where
  line : Nat
  checked : Bool
  key : String

def templateBoxes (t : List Line) : List TBox :=
  let th := usable t
  (indexed t).filterMap fun
    | (i, .box _ l) => some ⟨i, sectionAt th i, l.toLower⟩
    | _ => none

def bodyBoxes (b : List Line) : List BBox :=
  (indexed b).filterMap fun
    | (i, .box c l) => some ⟨i, c, l.toLower⟩
    | _ => none

/-- The section labels in order of first appearance. -/
def groupLabels (bs : List TBox) : List String :=
  bs.foldl (fun acc x => if acc.contains x.sec then acc else acc ++ [x.sec]) []

/-- Each template checkbox takes the first unused body checkbox with the same
key. The result is the body checkbox, or `none` if the body has none left. -/
def assign : List BBox → List String → List (Option BBox) × List BBox
  | rem, [] => ([], rem)
  | rem, k :: ks =>
    match takeFirst (fun x => x.key == k) rem with
    | some (x, rem') => let r := assign rem' ks; (some x :: r.1, r.2)
    | none => let r := assign rem ks; (none :: r.1, r.2)

/-- One checklist: its label, its item keys and the body checkbox that each item found. -/
structure Group where
  sec : String
  keys : List String
  found : List (Option BBox)

/-- A template checkbox is touched if the body checked it or removed it. -/
def touchedMatch : Option BBox → Bool
  | none => true
  | some x => x.checked

def Group.touched (g : Group) : Bool := g.found.any touchedMatch

/-- The untouched items: their key and their body checkbox. -/
def Group.untouched (g : Group) : List (String × BBox) :=
  (g.keys.zip g.found).filterMap fun
    | (k, some x) => if x.checked then none else some (k, x)
    | (_, none) => none

def groupsFrom : List BBox → List TBox → List String → List Group
  | _, _, [] => []
  | rem, bs, s :: ss =>
    let keys := (bs.filter (·.sec == s)).map (·.key)
    let r := assign rem keys
    ⟨s, keys, r.1⟩ :: groupsFrom r.2 bs ss

def groups (t b : List Line) : List Group :=
  let bs := templateBoxes t
  groupsFrom (bodyBoxes b) bs (groupLabels bs)

def groupFindings (o : Opts) (g : Group) : List Finding :=
  match g.untouched with
  | [] => []
  | (_, x) :: _ =>
    if !g.touched then
      [⟨if o.allowUnchecked then .warning else .error, "untouched-checklist", g.sec, x.line + 1, g.sec⟩]
    else
      g.untouched.map fun (k, y) => ⟨.warning, "unchecked-checkbox", g.sec, y.line + 1, k⟩

def checklistPart (t b : List Line) (o : Opts) : List Finding :=
  (groups t b).flatMap (groupFindings o)

def isBlank (b : List Line) : Bool := b.all (· == .blank)

def raise (o : Opts) (f : Finding) : Finding :=
  if o.strict && f.severity == .warning then { f with severity := .error } else f

/-- The model of `check.Markdown`. -/
def check (t b : List Line) (o : Opts) : List Finding :=
  let fs :=
    if isBlank b then [⟨.error, "empty-body", "", 0, ""⟩]
    else headingPart t b ++ commentPart t b ++ checklistPart t b o
  fs.map (raise o)

/-- The check passes if no finding is an error. -/
def passes (fs : List Finding) : Bool := fs.all (·.severity != .error)

/-! ## Theorems -/

theorem raise_code (o : Opts) (f : Finding) : (raise o f).code = f.code := by
  unfold raise; split <;> rfl

theorem raise_subject (o : Opts) (f : Finding) : (raise o f).subject = f.subject := by
  unfold raise; split <;> rfl

theorem raise_sec (o : Opts) (f : Finding) : (raise o f).sec = f.sec := by
  unfold raise; split <;> rfl

theorem raise_line (o : Opts) (f : Finding) : (raise o f).line = f.line := by
  unfold raise; split <;> rfl

theorem raise_error (o : Opts) (f : Finding) (h : f.severity = .error) :
    (raise o f).severity = .error := by
  unfold raise; split <;> simp_all

theorem raise_strict (o : Opts) (f : Finding) (h : o.strict = true) :
    (raise o f).severity = .error := by
  unfold raise
  cases hs : f.severity <;> simp [h, hs]

theorem check_nonblank (t b : List Line) (o : Opts) (hb : isBlank b = false) :
    check t b o = (headingPart t b ++ commentPart t b ++ checklistPart t b o).map (raise o) := by
  simp [check, hb]

/-- A passing check has no error finding. -/
theorem passes_no_error (fs : List Finding) (h : passes fs = true) (f : Finding)
    (hf : f ∈ fs) : f.severity ≠ .error := by
  simp only [passes, List.all_eq_true, bne_iff_ne, ne_eq] at h
  exact h f hf

theorem headingFinding_error (t b : List Line) (tEnds bEnds : List Nat)
    (x : (Nat × String) × Option Nat) (f : Finding)
    (hf : f ∈ headingFinding t b tEnds bEnds x) : f.severity = .error := by
  unfold headingFinding at hf
  split at hf
  · simp at hf; subst hf; rfl
  · simp only at hf
    split at hf
    · simp at hf
    · split at hf
      · simp at hf; subst hf; rfl
      · split at hf
        · simp at hf; subst hf; rfl
        · simp at hf

theorem headingFinding_missing (t b : List Line) (tEnds bEnds : List Nat)
    (x : (Nat × String) × Option Nat) (k : String) :
    ((headingFinding t b tEnds bEnds x).filter fun f => f.code == "missing-heading" && f.subject == k).length =
      if key x.1.2 == k && x.2.isNone then 1 else 0 := by
  obtain ⟨⟨tl, tx⟩, mi⟩ := x
  cases mi with
  | none => by_cases hk : key tx = k <;> simp [headingFinding, hk]
  | some bl =>
    simp only [headingFinding, Option.isNone_some, Bool.and_false]
    split <;> (try split) <;> (try split) <;> simp_all

/-- Every finding of `headingPart` with code `missing-heading` comes from an
unmatched template heading with the same key. -/
theorem headingPart_missing (t b : List Line) (k : String) :
    ((headingPart t b).filter fun f => f.code == "missing-heading" && f.subject == k).length =
      unmatched k ((usable t).map fun p => key p.2)
        (matchH ((headings b).map fun p => (p.1, key p.2)) ((usable t).map fun p => key p.2)) := by
  unfold headingPart unmatched
  simp only
  generalize matchH ((headings b).map fun p => (p.1, key p.2)) ((usable t).map fun p => key p.2) = m
  generalize (usable t).map (·.1) = tEnds
  generalize m.filterMap id = bEnds
  generalize usable t = th
  induction th generalizing m with
  | nil => simp
  | cons h th ih =>
    cases m with
    | nil => simp
    | cons mi m =>
      simp only [List.zip_cons_cons, List.flatMap_cons, List.filter_append, List.length_append,
        List.map_cons, List.filter_cons, ih, headingFinding_missing]
      split <;> simp_all <;> omega

/-- Exactly as many `missing-heading` findings as the body lacks. -/
theorem missing_heading_count (t b : List Line) (o : Opts) (hb : isBlank b = false) (k : String) :
    ((check t b o).filter fun f => f.code == "missing-heading" && f.subject == k).length =
      ((usable t).map fun p => key p.2).count k -
        ((headings b).map fun p => key p.2).count k := by
  rw [check_nonblank t b o hb, List.filter_map]
  simp only [List.length_map, Function.comp_def, raise_code, raise_subject, List.filter_append,
    List.length_append]
  have hc : ((commentPart t b).filter fun f => f.code == "missing-heading" && f.subject == k) = [] := by
    simp only [List.filter_eq_nil_iff, Bool.and_eq_true, beq_iff_eq, not_and]
    intro f hf
    simp only [commentPart, List.mem_filterMap] at hf
    obtain ⟨⟨i, l⟩, _, hl⟩ := hf
    cases l <;> simp at hl
    obtain ⟨_, rfl⟩ := hl; simp
  have hl : ((checklistPart t b o).filter fun f => f.code == "missing-heading" && f.subject == k) = [] := by
    simp only [List.filter_eq_nil_iff, Bool.and_eq_true, beq_iff_eq, not_and]
    intro f hf
    simp only [checklistPart, List.mem_flatMap] at hf
    obtain ⟨g, _, hg⟩ := hf
    unfold groupFindings at hg
    split at hg
    · simp at hg
    · split at hg
      · simp at hg; subst hg; simp
      · simp only [List.mem_map] at hg
        obtain ⟨_, _, rfl⟩ := hg
        simp
  rw [hc, hl, headingPart_missing, matchH_unmatched, List.countP_map]
  simp only [List.length_nil, Nat.add_zero]
  congr 1
  simp [List.count_eq_countP, List.countP_map, Function.comp_def]

/-- If the check passes, the body keeps every template heading: each key is in
the body at least as often as in the template. -/
theorem pass_keeps_headings (t b : List Line) (o : Opts) (hb : isBlank b = false)
    (h : passes (check t b o) = true) (k : String) :
    ((usable t).map fun p => key p.2).count k ≤
      ((headings b).map fun p => key p.2).count k := by
  have hn := missing_heading_count t b o hb k
  have hz : ((check t b o).filter fun f => f.code == "missing-heading" && f.subject == k) = [] := by
    simp only [List.filter_eq_nil_iff, Bool.and_eq_true, beq_iff_eq, not_and]
    intro f hf hcode _
    apply passes_no_error _ h f hf
    rw [check_nonblank t b o hb] at hf
    simp only [List.mem_map, List.mem_append] at hf
    obtain ⟨f', hf', rfl⟩ := hf
    apply raise_error
    rw [raise_code] at hcode
    rcases hf' with (hf' | hf') | hf'
    · simp only [headingPart, List.mem_flatMap] at hf'
      obtain ⟨_, _, hm⟩ := hf'
      exact headingFinding_error _ _ _ _ _ _ hm
    · simp only [commentPart, List.mem_filterMap] at hf'
      obtain ⟨⟨_, l⟩, _, hl⟩ := hf'
      cases l <;> simp at hl
      obtain ⟨_, rfl⟩ := hl; rfl
    · simp only [checklistPart, List.mem_flatMap] at hf'
      obtain ⟨g, _, hg⟩ := hf'
      unfold groupFindings at hg
      split at hg
      · simp at hg
      · split at hg
        · simp at hg; subst hg; simp at hcode
        · simp only [List.mem_map] at hg
          obtain ⟨_, _, rfl⟩ := hg
          simp at hcode
  rw [hz] at hn
  simp at hn
  omega

/-- Each template comment that is still in the body is reported at its line. -/
theorem placeholder_reported (t b : List Line) (o : Opts) (hb : isBlank b = false)
    (i : Nat) (c : String) (hi : b[i]? = some (.comment c)) (hp : c ∈ placeholders t) :
    ∃ f ∈ check t b o, f.code = "placeholder-comment" ∧ f.line = i + 1 ∧
      f.subject = c ∧ f.severity = .error := by
  rw [check_nonblank t b o hb]
  refine ⟨raise o ⟨.error, "placeholder-comment", sectionAt (headings b) i, i + 1, c⟩, ?_, ?_⟩
  · apply List.mem_map_of_mem
    apply List.mem_append_left
    apply List.mem_append_right
    simp only [commentPart, List.mem_filterMap]
    refine ⟨(i, .comment c), ?_, ?_⟩
    · simp only [indexed, List.mem_map]
      refine ⟨(.comment c, i), ?_, rfl⟩
      rw [List.mem_zipIdx_iff_getElem?]
      simpa using hi
    · simp [hp]
  · exact ⟨raise_code _ _, raise_line _ _, raise_subject _ _, raise_error _ _ rfl⟩

/-- If the check passes, no template comment is left in the body. -/
theorem pass_has_no_placeholder (t b : List Line) (o : Opts) (hb : isBlank b = false)
    (h : passes (check t b o) = true) (i : Nat) (c : String)
    (hi : b[i]? = some (.comment c)) : c ∉ placeholders t := by
  intro hp
  obtain ⟨f, hf, _, _, _, he⟩ := placeholder_reported t b o hb i c hi hp
  exact passes_no_error _ h f hf he

theorem groupFindings_errors (o : Opts) (g : Group) (h : ∀ f ∈ groupFindings o g, f.severity ≠ .error)
    (hu : o.allowUnchecked = false) : g.untouched = [] ∨ g.touched = true := by
  unfold groupFindings at h
  split at h
  · left; assumption
  · right
    split at h
    · exfalso
      exact h _ (List.mem_singleton_self _) (by simp [hu])
    · rename_i hnt
      simpa using hnt

/-- The findings of a checklist part, raised, are in the check. -/
theorem group_in_check (t b : List Line) (o : Opts) (hb : isBlank b = false) (g : Group)
    (hg : g ∈ groups t b) (f : Finding) (hf : f ∈ groupFindings o g) : raise o f ∈ check t b o := by
  rw [check_nonblank t b o hb]
  apply List.mem_map_of_mem
  apply List.mem_append_right
  simp only [checklistPart, List.mem_flatMap]
  exact ⟨g, hg, hf⟩

/-- Every group has items. -/
theorem untouched_nil_touched (g : Group) (hk : g.keys.length = g.found.length)
    (hne : g.keys ≠ []) (hu : g.untouched = []) : g.touched = true := by
  obtain ⟨s, keys, ms⟩ := g
  simp only [Group.untouched, Group.touched] at *
  cases keys with
  | nil => exact absurd rfl hne
  | cons k ks =>
    cases ms with
    | nil => simp at hk
    | cons m ms =>
      simp only [List.zip_cons_cons, List.filterMap_cons] at hu
      cases m with
      | none => simp [touchedMatch]
      | some x =>
        cases hx : x.checked
        · simp [hx] at hu
        · simp [touchedMatch, hx]

theorem assign_length (rem : List BBox) (ks : List String) :
    (assign rem ks).1.length = ks.length := by
  induction ks generalizing rem with
  | nil => rfl
  | cons k ks ih =>
    unfold assign
    split <;> simp [ih]

theorem groupsFrom_shape (rem : List BBox) (bs : List TBox) (ss : List String) (g : Group)
    (hg : g ∈ groupsFrom rem bs ss) :
    g.keys.length = g.found.length ∧ g.keys = (bs.filter (·.sec == g.sec)).map (·.key) ∧
      g.sec ∈ ss := by
  induction ss generalizing rem with
  | nil => simp [groupsFrom] at hg
  | cons s ss ih =>
    simp only [groupsFrom, List.mem_cons] at hg
    rcases hg with rfl | hg
    · simp [assign_length]
    · obtain ⟨a, b, c⟩ := ih _ hg
      exact ⟨a, b, List.mem_cons_of_mem _ c⟩

theorem foldl_labels_mem (bs : List TBox) (s : String) (acc : List String)
    (h : s ∈ bs.foldl (fun acc x => if acc.contains x.sec then acc else acc ++ [x.sec]) acc) :
    s ∈ acc ∨ ∃ x ∈ bs, x.sec = s := by
  induction bs generalizing acc with
  | nil => left; simpa using h
  | cons y ys ih =>
    simp only [List.foldl_cons] at h
    rcases ih _ h with h1 | ⟨x, hx, hs⟩
    · split at h1
      · left; exact h1
      · simp only [List.mem_append, List.mem_singleton] at h1
        rcases h1 with h1 | rfl
        · left; exact h1
        · right; exact ⟨y, List.mem_cons_self, rfl⟩
    · right; exact ⟨x, List.mem_cons_of_mem _ hx, hs⟩

theorem groupLabels_mem (bs : List TBox) (s : String) (h : s ∈ groupLabels bs) :
    ∃ x ∈ bs, x.sec = s := by
  rcases foldl_labels_mem bs s [] h with h1 | h1
  · simp at h1
  · exact h1

/-- A group always has at least one item. -/
theorem group_keys_ne (t b : List Line) (g : Group) (hg : g ∈ groups t b) : g.keys ≠ [] := by
  obtain ⟨_, hk, hs⟩ := groupsFrom_shape _ _ _ g hg
  obtain ⟨x, hx, hxs⟩ := groupLabels_mem _ _ hs
  rw [hk]
  intro hnil
  simp only [List.map_eq_nil_iff, List.filter_eq_nil_iff, beq_iff_eq] at hnil
  exact hnil x hx hxs

/-- If the check passes without `--allow-unchecked`, every template checklist
has an item that the body checked or removed. -/
theorem pass_touches_checklists (t b : List Line) (o : Opts) (hb : isBlank b = false)
    (hu : o.allowUnchecked = false) (h : passes (check t b o) = true) (g : Group)
    (hg : g ∈ groups t b) : g.touched = true := by
  have he : ∀ f ∈ groupFindings o g, f.severity ≠ .error := by
    intro f hf hse
    exact passes_no_error _ h _ (group_in_check t b o hb g hg f hf) (raise_error _ _ hse)
  rcases groupFindings_errors o g he hu with hn | ht
  · exact untouched_nil_touched g (groupsFrom_shape _ _ _ g hg).1 (group_keys_ne t b g hg) hn
  · exact ht

/-- Each untouched checkbox is in a finding: either the checklist is reported
as untouched, or the checkbox is reported on its own. -/
theorem untouched_reported (t b : List Line) (o : Opts) (hb : isBlank b = false) (g : Group)
    (hg : g ∈ groups t b) (k : String) (x : BBox) (hx : (k, x) ∈ g.untouched) :
    ∃ f ∈ check t b o, f.sec = g.sec ∧
      ((f.code = "untouched-checklist" ∧ g.touched = false) ∨
       (f.code = "unchecked-checkbox" ∧ f.line = x.line + 1 ∧ f.subject = k)) := by
  have hgf : ∀ f ∈ groupFindings o g, raise o f ∈ check t b o := group_in_check t b o hb g hg
  unfold groupFindings at hgf
  split at hgf
  · rename_i hnil; rw [hnil] at hx; simp at hx
  · split at hgf
    · rename_i hnt
      refine ⟨_, hgf _ (List.mem_singleton_self _), ?_, Or.inl ⟨?_, by simpa using hnt⟩⟩
      · exact raise_sec _ _
      · exact raise_code _ _
    · refine ⟨_, hgf _ (List.mem_map.mpr ⟨(k, x), hx, rfl⟩), ?_, Or.inr ⟨?_, ?_, ?_⟩⟩
      · exact raise_sec _ _
      · exact raise_code _ _
      · exact raise_line _ _
      · exact raise_subject _ _

/-- With `--strict`, a pass means every template checkbox is touched. -/
theorem strict_pass_touches_every_checkbox (t b : List Line) (o : Opts) (hb : isBlank b = false)
    (hs : o.strict = true) (h : passes (check t b o) = true) (g : Group)
    (hg : g ∈ groups t b) : g.untouched = [] := by
  cases hu : g.untouched with
  | nil => rfl
  | cons p ps =>
    exfalso
    obtain ⟨k, x⟩ := p
    obtain ⟨f, hf, _, _⟩ := untouched_reported t b o hb g hg k x (by rw [hu]; exact List.mem_cons_self)
    apply passes_no_error _ h f hf
    rw [check_nonblank t b o hb] at hf
    obtain ⟨f', _, rfl⟩ := List.mem_map.mp hf
    exact raise_strict _ _ hs

end GhTmpl.Check
