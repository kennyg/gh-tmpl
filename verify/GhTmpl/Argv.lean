/-!
# The argument split of `pr create` and `issue create`

`gh tmpl pr create` and `gh tmpl issue create` get one list of words. Some
words are for gh-tmpl: its `--tmpl-*` flags, their values, and `--help`.
The rest go to `gh pr create` or `gh issue create`.

This file models `splitCreate` and `scanCreate` in `internal/cli/create.go`.

The specification is a grammar of tokens. A token is one flag with its value,
one plain word, or `--` with every word after it. A flag that takes a value
takes the next word, whoever owns the flag. The theorems say:

* `words_tokenize`: the tokens hold every word of the input, in order, and
  nothing else. No word is dropped, split, joined or moved.
* `valid_tokenize`: the tokens follow the grammar, so a value is never read
  as a flag.
* `tokenize_unique`: no other token list follows the grammar and holds the
  same words. The grammar alone decides the split.
* `split_interleave`: the gh-tmpl words and the gh words interleave to the
  input, so each side keeps the input order.

`scan` models `scanCreate`, which finds the body in the gh words. It has no
theorems. The differential test checks it against the Go code.
-/

namespace GhTmpl.Argv

/-- The flags that decide how words pair up. -/
structure Table where
  /-- The long flags of gh-tmpl, without `--`, and whether each takes a value. -/
  ownLongs : List (String × Bool)
  /-- The long flags of gh that take a value, without `--`, with their short
  form if they have one. -/
  ghLongs : List (String × Option Char)

/-- The short gh flags that take a value. -/
def Table.ghShorts (t : Table) : List Char :=
  t.ghLongs.filterMap (·.2)

/-- `w` is a gh-tmpl flag: `-h`, `--name` or `--name=value`. -/
def Table.isOwn (t : Table) (w : String) : Bool :=
  match w.toList with
  | ['-', 'h'] => true
  | '-' :: '-' :: rest => t.ownLongs.any (·.1 == String.ofList (rest.takeWhile (· != '=')))
  | _ => false

/-- `w` is a gh-tmpl flag that takes the next word as its value. -/
def Table.ownNeedsValue (t : Table) (w : String) : Bool :=
  match w.toList with
  | '-' :: '-' :: rest => !rest.contains '=' && t.ownLongs.contains (String.ofList rest, true)
  | _ => false

/-- A cluster of short flags takes the next word if its first short flag that
takes a value is its last character. -/
def clusterNeedsValue (shorts : List Char) : List Char → Bool
  | [] => false
  | c :: cs => if shorts.contains c then cs.isEmpty else clusterNeedsValue shorts cs

/-- `w` is a gh flag that takes the next word as its value. -/
def Table.ghNeedsValue (t : Table) (w : String) : Bool :=
  match w.toList with
  | '-' :: '-' :: rest => !rest.contains '=' && (t.ghLongs.map (·.1)).contains (String.ofList rest)
  | '-' :: cs => clusterNeedsValue t.ghShorts cs
  | _ => false

/-- One flag with its value, one plain word, or `--` and the words after it. -/
inductive Token where
  /-- A gh-tmpl flag and its value. -/
  | own (flag : String) (value : Option String)
  /-- A gh word. If it is a flag that takes a value, its value. -/
  | gh (word : String) (value : Option String)
  /-- `--` and every word after it. They all go to gh. -/
  | rest (words : List String)
  deriving Repr, DecidableEq

/-- The words of a token, in order. -/
def Token.words : Token → List String
  | .own f none | .gh f none => [f]
  | .own f (some v) | .gh f (some v) => [f, v]
  | .rest ws => ws

/-- The token goes to gh-tmpl. -/
def Token.isOwn : Token → Bool
  | .own .. => true
  | _ => false

/-- The grammar for one token. `last` tells whether the token ends the list.
A flag that takes a value has its value, unless it is the last word. -/
def Token.ok (t : Table) (last : Bool) : Token → Bool
  | .rest ws => last && ws.head? == some "--"
  | .own f v =>
    f != "--" && t.isOwn f &&
      match v with
      | some _ => t.ownNeedsValue f
      | none => !t.ownNeedsValue f || last
  | .gh f v =>
    f != "--" && !t.isOwn f &&
      match v with
      | some _ => t.ghNeedsValue f
      | none => !t.ghNeedsValue f || last

/-- The grammar for a token list. -/
def valid (t : Table) : List Token → Bool
  | [] => true
  | x :: xs => x.ok t xs.isEmpty && valid t xs

/-- The model of `splitCreate`: read the words into tokens. -/
def tokenize (t : Table) : List String → List Token
  | [] => []
  | w :: ws =>
    if w = "--" then [.rest (w :: ws)]
    else if t.isOwn w then
      if t.ownNeedsValue w then
        match ws with
        | v :: ws' => .own w (some v) :: tokenize t ws'
        | [] => [.own w none]
      else .own w none :: tokenize t ws
    else if t.ghNeedsValue w then
      match ws with
      | v :: ws' => .gh w (some v) :: tokenize t ws'
      | [] => [.gh w none]
    else .gh w none :: tokenize t ws

/-- The words for gh-tmpl and the words for gh. -/
def split (t : Table) (args : List String) : List String × List String :=
  let ts := tokenize t args
  ((ts.filter Token.isOwn).flatMap Token.words,
   (ts.filter (fun x => !x.isOwn)).flatMap Token.words)

/-! ## Theorems -/

@[simp] theorem tokenize_nil (t : Table) : tokenize t [] = [] := by
  rw [tokenize.eq_def]

theorem words_tokenize (t : Table) (args : List String) :
    (tokenize t args).flatMap Token.words = args := by
  fun_induction tokenize t args <;> simp_all [Token.words]

theorem valid_tokenize (t : Table) (args : List String) :
    valid t (tokenize t args) = true := by
  fun_induction tokenize t args <;> simp_all [valid, Token.ok]

/-- A valid token list that is not empty holds at least one word in each
token, so a valid list for no words is empty. -/
theorem valid_nil_words (t : Table) (ts : List Token) (hv : valid t ts = true)
    (hw : ts.flatMap Token.words = []) : ts = [] := by
  cases ts with
  | nil => rfl
  | cons x xs =>
    exfalso
    cases x with
    | own f v => cases v <;> simp [Token.words] at hw
    | gh f v => cases v <;> simp [Token.words] at hw
    | rest ws =>
      simp only [valid, Token.ok, Bool.and_eq_true, beq_iff_eq] at hv
      simp only [List.flatMap_cons, Token.words, List.append_eq_nil_iff] at hw
      simp [hw.1] at hv

theorem tokenize_unique (t : Table) (ts : List Token) (args : List String)
    (hv : valid t ts = true) (hw : ts.flatMap Token.words = args) :
    ts = tokenize t args := by
  induction ts generalizing args with
  | nil => subst hw; rfl
  | cons x xs ih =>
    simp only [valid, Bool.and_eq_true] at hv
    obtain ⟨hx, hxs⟩ := hv
    have ihx := ih (xs.flatMap Token.words) hxs rfl
    subst hw
    cases x with
    | rest ws =>
      simp only [Token.ok, Bool.and_eq_true, List.isEmpty_iff, beq_iff_eq] at hx
      obtain ⟨hnil, hhead⟩ := hx
      subst hnil
      cases ws with
      | nil => simp at hhead
      | cons w ws =>
        simp at hhead
        subst hhead
        simp only [List.flatMap_cons, Token.words, List.cons_append]; rw [tokenize.eq_def]; simp
    | own f v =>
      simp only [Token.ok, Bool.and_eq_true, bne_iff_ne, ne_eq] at hx
      obtain ⟨⟨hf, hown⟩, hval⟩ := hx
      cases v with
      | some v =>
        simp only [List.flatMap_cons, Token.words, List.cons_append]; rw [tokenize.eq_def]; simp [ hf, hown, hval, ← ihx]
      | none =>
        simp only [Bool.or_eq_true, Bool.not_eq_true', List.isEmpty_iff] at hval
        rcases hval with hn | hlast
        · simp only [List.flatMap_cons, Token.words, List.cons_append]; rw [tokenize.eq_def]; simp [ hf, hown, hn, ← ihx]
        · subst hlast
          simp only [List.flatMap_cons, List.flatMap_nil, Token.words, List.append_nil]
          rw [tokenize.eq_def]
          cases hn : t.ownNeedsValue f <;> simp [hf, hown, hn]
    | gh f v =>
      simp only [Token.ok, Bool.and_eq_true, bne_iff_ne, ne_eq, Bool.not_eq_true'] at hx
      obtain ⟨⟨hf, hown⟩, hval⟩ := hx
      cases v with
      | some v =>
        simp only [List.flatMap_cons, Token.words, List.cons_append]; rw [tokenize.eq_def]; simp [ hf, hown, hval, ← ihx]
      | none =>
        simp only [Bool.or_eq_true, Bool.not_eq_true', List.isEmpty_iff] at hval
        rcases hval with hn | hlast
        · simp only [List.flatMap_cons, Token.words, List.cons_append]; rw [tokenize.eq_def]; simp [ hf, hown, hn, ← ihx]
        · subst hlast
          simp only [List.flatMap_cons, List.flatMap_nil, Token.words, List.append_nil]
          rw [tokenize.eq_def]
          cases hn : t.ghNeedsValue f <;> simp [hf, hown, hn]

/-- `zs` is `xs` and `ys` shuffled together, each in its own order. -/
inductive Interleave {α : Type} : List α → List α → List α → Prop where
  | nil : Interleave [] [] []
  | left {a xs ys zs} : Interleave xs ys zs → Interleave (a :: xs) ys (a :: zs)
  | right {a xs ys zs} : Interleave xs ys zs → Interleave xs (a :: ys) (a :: zs)

theorem Interleave.append_left {α : Type} {xs ys zs : List α} (w : List α)
    (h : Interleave xs ys zs) : Interleave (w ++ xs) ys (w ++ zs) := by
  induction w with
  | nil => simpa using h
  | cons a w ih => exact .left ih

theorem Interleave.append_right {α : Type} {xs ys zs : List α} (w : List α)
    (h : Interleave xs ys zs) : Interleave xs (w ++ ys) (w ++ zs) := by
  induction w with
  | nil => simpa using h
  | cons a w ih => exact .right ih

theorem Interleave.sublist {α : Type} {xs ys zs : List α} (h : Interleave xs ys zs) :
    xs.Sublist zs ∧ ys.Sublist zs := by
  induction h with
  | nil => simp
  | left _ ih => exact ⟨ih.1.cons_cons _, ih.2.cons _⟩
  | right _ ih => exact ⟨ih.1.cons _, ih.2.cons_cons _⟩

theorem Interleave.length {α : Type} {xs ys zs : List α} (h : Interleave xs ys zs) :
    xs.length + ys.length = zs.length := by
  induction h <;> simp <;> omega

theorem interleave_tokens (ts : List Token) :
    Interleave ((ts.filter Token.isOwn).flatMap Token.words)
      ((ts.filter (fun x => !x.isOwn)).flatMap Token.words) (ts.flatMap Token.words) := by
  induction ts with
  | nil => exact .nil
  | cons x xs ih =>
    cases hx : x.isOwn
    · simpa [List.filter_cons, hx] using ih.append_right x.words
    · simpa [List.filter_cons, hx] using ih.append_left x.words

/-- The gh-tmpl words and the gh words interleave to the input. -/
theorem split_interleave (t : Table) (args : List String) :
    Interleave (split t args).1 (split t args).2 args := by
  have h := interleave_tokens (tokenize t args)
  rwa [words_tokenize] at h

/-- Each side keeps the input order, and together they hold every word. -/
theorem split_keeps_words (t : Table) (args : List String) :
    (split t args).1.Sublist args ∧ (split t args).2.Sublist args ∧
      (split t args).1.length + (split t args).2.length = args.length :=
  let h := split_interleave t args
  ⟨h.sublist.1, h.sublist.2, h.length⟩

/-! ## The body scan -/

/-- What `scanCreate` finds in the gh words. -/
structure Scan where
  body : Option String := none
  bodyFile : Option String := none
  repo : Option String := none
  deriving Repr, DecidableEq

def Scan.record (s : Scan) (long value : String) : Scan :=
  match long with
  | "body" => { s with body := some value }
  | "body-file" => { s with bodyFile := some value }
  | "repo" => { s with repo := some value }
  | _ => s

/-- The first short flag in a cluster that takes a value: its long name and
the rest of the cluster. -/
def firstValued (t : Table) : List Char → Option (String × List Char)
  | [] => none
  | c :: cs =>
    match t.ghLongs.find? (·.2 == some c) with
    | some (long, _) => some (long, cs)
    | none => firstValued t cs

/-- The model of `scanCreate`. `none` means a flag at the end has no value. -/
def scan (t : Table) : List String → Scan → Option Scan
  | [], s => some s
  | w :: ws, s =>
    if w = "--" then some s
    else
      match w.toList with
      | '-' :: '-' :: rest =>
        let name := String.ofList (rest.takeWhile (· != '='))
        if !(t.ghLongs.map (·.1)).contains name then scan t ws s
        else if rest.contains '=' then
          scan t ws (s.record name (String.ofList ((rest.dropWhile (· != '=')).drop 1)))
        else
          match ws with
          | v :: ws' => scan t ws' (s.record name v)
          | [] => none
      | '-' :: c :: cs =>
        match firstValued t (c :: cs) with
        | none => scan t ws s
        | some (long, after) =>
          if after.isEmpty then
            match ws with
            | v :: ws' => scan t ws' (s.record long v)
            | [] => none
          else
            let value := match after with
              | '=' :: r => r
              | r => r
            scan t ws (s.record long (String.ofList value))
      | _ => scan t ws s

end GhTmpl.Argv
