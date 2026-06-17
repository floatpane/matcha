use std::collections::HashSet;
use std::ffi::{CStr, CString};
use std::fs::File;
use std::io::{BufRead, BufReader};
use std::os::raw::{c_char, c_int};

pub struct SpellDict {
    words: HashSet<String>,
    runes: HashSet<char>,
}

impl SpellDict {
    fn parse(path: &str) -> Option<Self> {
        let file = File::open(path).ok()?;
        let reader = BufReader::with_capacity(256 * 1024, file);

        let mut words = HashSet::with_capacity(50_000);
        let mut runes = HashSet::with_capacity(64);
        let mut first = true;

        for line in reader.lines() {
            let raw = line.ok()?;
            let line = raw.trim();
            if line.is_empty() || line.starts_with('#') {
                continue;
            }
            if first {
                first = false;
                // Skip a bare count line (no spaces/tabs, parses as integer).
                if !line.contains(|c: char| c == ' ' || c == '\t')
                    && line.parse::<u64>().is_ok()
                {
                    continue;
                }
            }
            // Strip Hunspell /FLAGS suffix and tab-delimited metadata.
            let word = line
                .split('/')
                .next()
                .unwrap_or(line)
                .split('\t')
                .next()
                .unwrap_or(line)
                .trim();
            if word.is_empty() {
                continue;
            }
            let lower = word.to_lowercase();
            for c in lower.chars() {
                if is_dict_letter(c) {
                    runes.insert(c);
                }
            }
            words.insert(lower);
        }

        Some(SpellDict { words, runes })
    }

    fn contains(&self, word: &str) -> bool {
        let lower = word.to_lowercase();
        if self.words.contains(&lower) {
            return true;
        }
        // Accept possessives/contractions by stripping apostrophe suffix.
        if let Some(idx) = lower.find('\'') {
            if self.words.contains(&lower[..idx]) {
                return true;
            }
        }
        false
    }

    fn covers(&self, word: &str) -> bool {
        if self.runes.is_empty() {
            return true;
        }
        for c in word.chars() {
            if c.is_alphabetic() {
                let lc = c.to_lowercase().next().unwrap_or(c);
                if !self.runes.contains(&lc) {
                    return false;
                }
            }
        }
        true
    }

    fn suggest(&self, word: &str, limit: usize) -> Vec<String> {
        let lower = word.to_lowercase();
        let w_chars: Vec<char> = lower.chars().collect();
        if w_chars.len() < 2 {
            return Vec::new();
        }
        let max_dist = if w_chars.len() >= 8 { 3 } else { 2 };

        let mut cands: Vec<(String, usize)> = Vec::new();

        for w in &self.words {
            let len_diff = (w.chars().count() as isize - w_chars.len() as isize).unsigned_abs();
            if len_diff > max_dist {
                continue;
            }
            if !first_rune_close(w, &lower) {
                continue;
            }
            let b_chars: Vec<char> = w.chars().collect();
            let d = levenshtein(&w_chars, &b_chars, max_dist);
            if d <= max_dist {
                cands.push((w.clone(), d));
            }
        }

        cands.sort_by(|a, b| a.1.cmp(&b.1).then(a.0.cmp(&b.0)));
        cands.truncate(limit);
        cands.into_iter().map(|(w, _)| w).collect()
    }
}

fn is_dict_letter(c: char) -> bool {
    if c.is_ascii() {
        c.is_ascii_alphabetic()
    } else {
        c.is_alphabetic()
    }
}

fn first_rune_close(a: &str, b: &str) -> bool {
    match (a.chars().next(), b.chars().next()) {
        (None, _) | (_, None) => true,
        (Some(ac), Some(bc)) if ac == bc => true,
        (Some(ac), Some(bc)) => keyboard_adjacent(ac, bc),
    }
}

fn keyboard_adjacent(a: char, b: char) -> bool {
    const ROWS: &[(char, &str)] = &[
        ('a', "qwsz"),   ('b', "vghn"),   ('c', "xdfv"),  ('d', "serfcx"),
        ('e', "wsdr"),   ('f', "drtgcv"), ('g', "ftyhvb"), ('h', "gyujnb"),
        ('i', "ujko"),   ('j', "huikmn"), ('k', "jiolm"),  ('l', "kop"),
        ('m', "njk"),    ('n', "bhjm"),   ('o', "iklp"),   ('p', "ol"),
        ('q', "wa"),     ('r', "edft"),   ('s', "awedxz"), ('t', "rfgy"),
        ('u', "yhji"),   ('v', "cfgb"),   ('w', "qase"),   ('x', "zsdc"),
        ('y', "tghu"),   ('z', "asx"),
    ];
    let a = a.to_lowercase().next().unwrap_or(a);
    let b = b.to_lowercase().next().unwrap_or(b);
    for (k, ns) in ROWS {
        if *k == a {
            return ns.contains(b);
        }
    }
    false
}

fn levenshtein(a: &[char], b: &[char], cutoff: usize) -> usize {
    let (la, lb) = (a.len(), b.len());
    if la == 0 {
        return lb;
    }
    if lb == 0 {
        return la;
    }
    let mut prev: Vec<usize> = (0..=lb).collect();
    let mut curr = vec![0usize; lb + 1];
    for i in 1..=la {
        curr[0] = i;
        let mut min_row = curr[0];
        for j in 1..=lb {
            let cost = usize::from(a[i - 1] != b[j - 1]);
            curr[j] = (curr[j - 1] + 1).min(prev[j] + 1).min(prev[j - 1] + cost);
            if curr[j] < min_row {
                min_row = curr[j];
            }
        }
        if min_row > cutoff {
            return cutoff + 1;
        }
        std::mem::swap(&mut prev, &mut curr);
    }
    prev[lb]
}

// ── C FFI ────────────────────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn spelldict_load(path: *const c_char) -> *mut SpellDict {
    if path.is_null() {
        return std::ptr::null_mut();
    }
    let path_str = unsafe { CStr::from_ptr(path) };
    match path_str.to_str().ok().and_then(SpellDict::parse) {
        Some(d) => Box::into_raw(Box::new(d)),
        None => std::ptr::null_mut(),
    }
}

#[no_mangle]
pub extern "C" fn spelldict_free(dict: *mut SpellDict) {
    if !dict.is_null() {
        unsafe { drop(Box::from_raw(dict)) };
    }
}

#[no_mangle]
pub extern "C" fn spelldict_contains(
    dict: *const SpellDict,
    word: *const c_char,
    len: usize,
) -> c_int {
    if dict.is_null() || word.is_null() {
        return 0;
    }
    let bytes = unsafe { std::slice::from_raw_parts(word as *const u8, len) };
    match std::str::from_utf8(bytes) {
        Ok(s) => c_int::from(unsafe { &*dict }.contains(s)),
        Err(_) => 0,
    }
}

#[no_mangle]
pub extern "C" fn spelldict_covers(
    dict: *const SpellDict,
    word: *const c_char,
    len: usize,
) -> c_int {
    if dict.is_null() || word.is_null() {
        return 1;
    }
    let bytes = unsafe { std::slice::from_raw_parts(word as *const u8, len) };
    match std::str::from_utf8(bytes) {
        Ok(s) => c_int::from(unsafe { &*dict }.covers(s)),
        Err(_) => 1,
    }
}

/// Returns a NULL-terminated array of C strings. Free with `spelldict_free_suggestions`.
#[no_mangle]
pub extern "C" fn spelldict_suggest(
    dict: *const SpellDict,
    word: *const c_char,
    len: usize,
    limit: c_int,
) -> *mut *mut c_char {
    if dict.is_null() || word.is_null() {
        return std::ptr::null_mut();
    }
    let bytes = unsafe { std::slice::from_raw_parts(word as *const u8, len) };
    let word_str = match std::str::from_utf8(bytes) {
        Ok(s) => s,
        Err(_) => return std::ptr::null_mut(),
    };
    let lim = if limit <= 0 { 5 } else { limit as usize };
    let suggestions = unsafe { &*dict }.suggest(word_str, lim);

    let mut ptrs: Vec<*mut c_char> = suggestions
        .into_iter()
        .filter_map(|s| CString::new(s).ok().map(|cs| cs.into_raw()))
        .collect();
    ptrs.push(std::ptr::null_mut());

    // into_boxed_slice shrinks to exact length so Box::from_raw can reconstruct it.
    let mut boxed = ptrs.into_boxed_slice();
    let ptr = boxed.as_mut_ptr();
    std::mem::forget(boxed);
    ptr
}

#[no_mangle]
pub extern "C" fn spelldict_free_suggestions(suggestions: *mut *mut c_char) {
    if suggestions.is_null() {
        return;
    }
    unsafe {
        let mut count = 0usize;
        while !(*suggestions.add(count)).is_null() {
            drop(CString::from_raw(*suggestions.add(count)));
            count += 1;
        }
        // count+1 = length used by Box::leak (includes the NULL terminator).
        drop(Box::from_raw(std::slice::from_raw_parts_mut(
            suggestions,
            count + 1,
        )));
    }
}
