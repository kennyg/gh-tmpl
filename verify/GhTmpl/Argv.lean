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

A switch is a gh-tmpl flag that takes no value. Its word is `--name`, or
`--name=value` with one of the values true, false, 1 and 0. `switchWord`
reads the word, and `switchOn` says what the parser binds. The theorems say:

* `switchWord_bare` and `switchWord_attached`: `--name` sets the switch to
  true, and `--name=value` sets it to `boolValue value`, never to true for
  false or 0.
* `boolValue_isSome`: the four values are the only ones, and
  `parses_bad_value`: any other value is a usage error.
* `switchWord_own` and `tokenize_switch`: a switch word, with or without its
  value, is one gh-tmpl token and never takes the next word.
* `switchOn_last`: the last word that sets a switch wins, so
  `--tmpl-skip --tmpl-skip=false` leaves the check on.

`scan` models `scanCreate`, which finds the body in the gh words. `tmplName`
and `parses` say what the generated parser reads from the gh-tmpl words.
Apart from `parses_bad_value`, they have no theorems. The differential test
checks them and `switchOn` against the Go code.
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
  | '-' :: '-' :: rest => !rest.contains '=' && t.ownLongs.lookup (String.ofList rest) == some true
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

/-! ## What the parser reads from the gh-tmpl words -/

/-- The value after the first `=` of a long flag, if the flag is `--name=value`. -/
def attached (name : String) (w : String) : Option String :=
  match w.toList with
  | '-' :: '-' :: rest =>
    if String.ofList (rest.takeWhile (· != '=')) == name && rest.contains '=' then
      some (String.ofList ((rest.dropWhile (· != '=')).drop 1))
    else none
  | _ => none

/-- The value of the last `--tmpl-name`. -/
def tmplName (t : Table) (args : List String) : Option String :=
  (tokenize t args).foldl (fun acc tok =>
    match tok with
    | .own f (some v) => if f == "--tmpl-name" then some v else acc
    | .own f none => (attached "tmpl-name" f).orElse fun _ => acc
    | _ => acc) none

/-! ## The switches

A switch is a gh-tmpl flag that takes no value, such as `--tmpl-skip`. It
takes no next word, but it can have an attached value, as in
`--tmpl-skip=false`. `switchValues` in `internal/cli/cli.go` reads the four
values below and rejects any other, and the generated parser binds the value
of the last word that sets the switch. -/

/-- The value of a switch. `none` is a usage error. -/
def boolValue : String → Option Bool
  | "true" | "1" => some true
  | "false" | "0" => some false
  | _ => none

/-- `n` is a gh-tmpl switch. `--help` is not one. -/
def Table.isSwitch (t : Table) (n : String) : Bool :=
  n != "help" && t.ownLongs.lookup n == some false

/-- The switch that a word sets, and its value: `--n` sets `n` to true, and
`--n=v` sets `n` to `boolValue v`. `none` if the word is not a switch. -/
def switchWord (t : Table) (w : String) : Option (String × Option Bool) :=
  if w = "--" then none
  else
    match w.toList with
    | '-' :: '-' :: rest =>
      let name := String.ofList (rest.takeWhile (· != '='))
      if !t.isSwitch name then none
      else if rest.contains '=' then
        some (name, boolValue (String.ofList ((rest.dropWhile (· != '=')).drop 1)))
      else some (name, some true)
    | _ => none

/-- The value that a token gives switch `n`, if it sets `n`. -/
def Token.sets (t : Table) (n : String) : Token → Option Bool
  | .own f none =>
    match switchWord t f with
    | some (m, some b) => if m == n then some b else none
    | _ => none
  | _ => none

/-- The value that the parser binds to switch `n`: the value of the last token
that sets it, or false. -/
def switchOn (t : Table) (n : String) (args : List String) : Bool :=
  (tokenize t args).foldl (fun acc tok => (tok.sets t n).getD acc) false

/-- The parser accepts the gh-tmpl words: they ask for no help, no flag at
the end lacks its value, and each switch has one of the four values. -/
def parses (t : Table) (args : List String) : Bool :=
  (tokenize t args).all fun
    | .own f none =>
      !(f == "-h" || f == "--help" || t.ownNeedsValue f) &&
        match switchWord t f with
        | some (_, none) => false
        | _ => true
    | _ => true

/-! ### Theorems about the switches -/

theorem boolValue_isSome (v : String) :
    (boolValue v).isSome = true ↔ v ∈ ["true", "1", "false", "0"] := by
  unfold boolValue
  split <;> simp_all

theorem any_of_lookup {l : List (String × Bool)} {n : String} {b : Bool}
    (h : l.lookup n = some b) : l.any (·.1 == n) = true := by
  induction l with
  | nil => simp at h
  | cons x xs ih =>
    obtain ⟨k, v⟩ := x
    simp only [List.lookup] at h
    split at h
    next he => simp_all
    next => simp [ih h]

theorem takeWhile_of_not_mem : ∀ {l : List Char}, '=' ∉ l → l.takeWhile (· != '=') = l
  | [], _ => rfl
  | c :: cs, h => by
    have hc : c ≠ '=' := fun he => h (he ▸ List.mem_cons_self ..)
    simp [hc, takeWhile_of_not_mem (fun hm => h (List.mem_cons_of_mem _ hm))]

/-- A switch word is a gh-tmpl word that takes no next word. -/
theorem switchWord_own (t : Table) (w : String) (h : (switchWord t w).isSome = true) :
    w ≠ "--" ∧ t.isOwn w = true ∧ t.ownNeedsValue w = false := by
  unfold switchWord at h
  by_cases hd : w = "--"
  · simp [hd] at h
  simp only [hd, ite_false] at h
  refine ⟨hd, ?_⟩
  unfold Table.isOwn Table.ownNeedsValue
  split at h
  next rest hw =>
    rw [hw]
    simp only [Table.isSwitch, Bool.not_eq_true'] at h
    split at h
    · simp at h
    next hs =>
      simp only [Bool.not_eq_false, Bool.and_eq_true, bne_iff_ne, ne_eq, beq_iff_eq] at hs
      refine ⟨any_of_lookup hs.2, ?_⟩
      by_cases he : '=' ∈ rest
      · simp [he]
      · rw [takeWhile_of_not_mem he] at hs
        simp [he, hs.2]
  next => simp at h

theorem tokenize_switch (t : Table) (w : String) (h : (switchWord t w).isSome = true) :
    tokenize t [w] = [.own w none] := by
  obtain ⟨hd, ho, hn⟩ := switchWord_own t w h
  rw [tokenize.eq_def]
  simp [hd, ho, hn]

/-- `--n` sets switch `n` to true. -/
theorem switchWord_bare (t : Table) (w n : String) (hw : w.toList = '-' :: '-' :: n.toList)
    (hne : n.toList ≠ []) (hn : '=' ∉ n.toList) (hs : t.isSwitch n = true) :
    switchWord t w = some (n, some true) := by
  have hd : w ≠ "--" := by
    intro he; subst he; simp at hw; exact hne (by simpa using hw.symm)
  unfold switchWord
  simp only [hd, ↓reduceIte]
  rw [hw]
  simp only [takeWhile_of_not_mem hn, String.ofList_toList, hs]
  simp [hn]

/-- `--n=v` sets switch `n` to `boolValue v`. -/
theorem switchWord_attached (t : Table) (w n v : String)
    (hw : w.toList = '-' :: '-' :: (n.toList ++ '=' :: v.toList))
    (hn : '=' ∉ n.toList) (hs : t.isSwitch n = true) :
    switchWord t w = some (n, boolValue v) := by
  have hd : w ≠ "--" := by
    intro he; subst he; simp at hw
  have hpos : ∀ c ∈ n.toList, (c != '=') = true := by
    intro c hc
    have : c ≠ '=' := fun he => hn (he ▸ hc)
    simpa using this
  unfold switchWord
  simp only [hd, ↓reduceIte]
  rw [hw]
  simp [List.takeWhile_append_of_pos hpos, List.dropWhile_append_of_pos hpos, hs]

/-- A switch with a value outside the four is a usage error. -/
theorem parses_bad_value (t : Table) (args : List String) (f n : String)
    (hm : Token.own f none ∈ tokenize t args) (hf : switchWord t f = some (n, none)) :
    parses t args = false := by
  unfold parses
  rw [List.all_eq_false]
  exact ⟨_, hm, by simp [hf]⟩

/-! ### The last word wins -/

/-- The token takes no word after it: it is not `--` with the rest, and not a
flag at the end that lacks its value. -/
def Token.closed (t : Table) : Token → Bool
  | .rest _ => false
  | .own f none => !t.ownNeedsValue f
  | .gh f none => !t.ghNeedsValue f
  | _ => true

/-- The last token of the list takes no word after it. -/
def closed (t : Table) : List Token → Bool
  | [] => true
  | [x] => x.closed t
  | _ :: x :: xs => closed t (x :: xs)

theorem closed_tail (t : Table) (x : Token) (xs : List Token) (h : closed t (x :: xs) = true) :
    closed t xs = true := by
  cases xs with
  | nil => rfl
  | cons y ys => simpa [closed] using h

/-- After words whose last token is closed, the tokens of more words follow
the tokens of the first words. -/
theorem tokenize_append (t : Table) (xs ys : List String) (h : closed t (tokenize t xs) = true) :
    tokenize t (xs ++ ys) = tokenize t xs ++ tokenize t ys := by
  fun_induction tokenize t xs <;> (try simp only [List.cons_append, List.nil_append]) <;>
    (try have hc := closed_tail _ _ _ h) <;> (conv => lhs; rw [tokenize.eq_def]) <;>
    simp_all [closed, Token.closed]

/-- A switch word at the end sets the switch, whatever the words before it
set, unless the word before it takes it as a value. So
`--tmpl-skip --tmpl-skip=false` leaves the check on. -/
theorem switchOn_last (t : Table) (xs : List String) (w n : String) (b : Bool)
    (hc : closed t (tokenize t xs) = true) (hw : switchWord t w = some (n, some b)) :
    switchOn t n (xs ++ [w]) = b := by
  unfold switchOn
  rw [tokenize_append t xs [w] hc, tokenize_switch t w (by simp [hw])]
  simp [Token.sets, hw]

end GhTmpl.Argv
