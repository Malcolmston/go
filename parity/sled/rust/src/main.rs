// Upstream (oracle) parity runner for sled 0.34.7.
//
// Reads one JSON object per line on stdin, writes exactly one JSON object per
// line on stdout. Never exits on a failing case: a failure is reported as
// {"ok": false, ...} and the loop continues.
//
// Each case is a SCRIPT of operations run against a freshly created database in
// a temp directory. The emitted value is
//   {"steps": [<result per step>], "trees": [{"name": hex, "entries": [[k,v]..]}]}
// so divergence in semantics (not just single return values) is visible.
//
// All keys, values and tree names are lowercase-hex encoded in JSON.

use serde_json::{json, Map, Value};
use std::collections::BTreeMap;
use std::io::{self, BufRead, Write};
use std::ops::Bound;
use std::path::PathBuf;

type R<T> = Result<T, String>;

fn hex_encode(b: &[u8]) -> String {
    let mut s = String::with_capacity(b.len() * 2);
    for byte in b {
        s.push_str(&format!("{:02x}", byte));
    }
    s
}

fn hex_decode(s: &str) -> R<Vec<u8>> {
    if s.len() % 2 != 0 {
        return Err(format!("odd hex length: {}", s));
    }
    let bytes = s.as_bytes();
    let mut out = Vec::with_capacity(s.len() / 2);
    let mut i = 0;
    while i < bytes.len() {
        let hi = (bytes[i] as char)
            .to_digit(16)
            .ok_or_else(|| format!("bad hex: {}", s))?;
        let lo = (bytes[i + 1] as char)
            .to_digit(16)
            .ok_or_else(|| format!("bad hex: {}", s))?;
        out.push(((hi << 4) | lo) as u8);
        i += 2;
    }
    Ok(out)
}

// ---------------------------------------------------------------- step fields

fn s_str(step: &Map<String, Value>, k: &str) -> Option<String> {
    step.get(k).and_then(|v| v.as_str()).map(|s| s.to_string())
}

fn s_bytes(step: &Map<String, Value>, k: &str) -> R<Option<Vec<u8>>> {
    match step.get(k) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::String(s)) => Ok(Some(hex_decode(s)?)),
        Some(other) => Err(format!("field {} must be a hex string, got {}", k, other)),
    }
}

fn s_req_bytes(step: &Map<String, Value>, k: &str) -> R<Vec<u8>> {
    s_bytes(step, k)?.ok_or_else(|| format!("missing field {}", k))
}

fn s_bool(step: &Map<String, Value>, k: &str, dflt: bool) -> bool {
    step.get(k).and_then(|v| v.as_bool()).unwrap_or(dflt)
}

fn s_u64(step: &Map<String, Value>, k: &str, dflt: u64) -> u64 {
    step.get(k).and_then(|v| v.as_u64()).unwrap_or(dflt)
}

fn opt_hex(v: Option<&[u8]>) -> Value {
    match v {
        None => Value::Null,
        Some(b) => Value::String(hex_encode(b)),
    }
}

fn pair(kv: Option<(sled::IVec, sled::IVec)>) -> Value {
    match kv {
        None => Value::Null,
        Some((k, v)) => json!([hex_encode(&k), hex_encode(&v)]),
    }
}

// ------------------------------------------------------------- merge operators

// The named merge operators must behave identically in both runners.
fn merge_apply(name: &str, _key: &[u8], old: Option<&[u8]>, merged: &[u8]) -> Option<Vec<u8>> {
    match name {
        // append the merged bytes to the current value
        "concat" => {
            let mut out = old.map(|o| o.to_vec()).unwrap_or_default();
            out.extend_from_slice(merged);
            Some(out)
        }
        // treat both as big-endian u64 counters and add
        "sum_u64" => {
            let cur = old.map(be64).unwrap_or(0);
            let add = be64(merged);
            Some(cur.wrapping_add(add).to_be_bytes().to_vec())
        }
        // a merge of the empty value deletes the key, otherwise replace
        "replace_or_delete" => {
            if merged.is_empty() {
                None
            } else {
                Some(merged.to_vec())
            }
        }
        _ => Some(merged.to_vec()),
    }
}

fn be64(b: &[u8]) -> u64 {
    let mut buf = [0u8; 8];
    let n = b.len().min(8);
    buf[8 - n..].copy_from_slice(&b[b.len() - n..]);
    u64::from_be_bytes(buf)
}

// ------------------------------------------------------------------ the state

const DEFAULT_TREE: &str = "__sled__default";

struct State {
    dir: PathBuf,
    db: sled::Db,
    // tree name -> merge operator name, reinstalled after a reopen so cases do
    // not have to (both runners do this identically).
    merge_ops: BTreeMap<String, String>,
    last_id: Option<u64>,
}

impl State {
    fn open(dir: PathBuf) -> R<State> {
        // NOTE: upstream sled takes a DIRECTORY path here. The Go port takes a
        // FILE path (it is an in-memory treap with a write-ahead log file).
        let db = sled::Config::new()
            .path(&dir)
            .open()
            .map_err(|e| e.to_string())?;
        Ok(State {
            dir,
            db,
            merge_ops: BTreeMap::new(),
            last_id: None,
        })
    }

    fn tree(&self, name: &str) -> R<sled::Tree> {
        if name.is_empty() || name == DEFAULT_TREE {
            return Ok((*self.db).clone());
        }
        self.db.open_tree(name.as_bytes()).map_err(|e| e.to_string())
    }

    fn install_merge(&self, name: &str, op: &str) -> R<()> {
        let t = self.tree(name)?;
        let op = op.to_string();
        t.set_merge_operator(move |k: &[u8], old: Option<&[u8]>, m: &[u8]| {
            merge_apply(&op, k, old, m)
        });
        Ok(())
    }

    fn reopen(&mut self) -> R<()> {
        self.db.flush().map_err(|e| e.to_string())?;
        let dir = self.dir.clone();
        let merge_ops = std::mem::take(&mut self.merge_ops);
        let last_id = self.last_id;
        // drop the old Db (releases the lock) by replacing self
        let fresh = {
            let placeholder = sled::Config::new()
                .temporary(true)
                .open()
                .map_err(|e| e.to_string())?;
            let old = std::mem::replace(&mut self.db, placeholder);
            drop(old);
            State::open(dir)?
        };
        self.db = fresh.db;
        self.merge_ops = merge_ops;
        self.last_id = last_id;
        let ops: Vec<(String, String)> = self
            .merge_ops
            .iter()
            .map(|(a, b)| (a.clone(), b.clone()))
            .collect();
        for (tree, op) in ops {
            self.install_merge(&tree, &op)?;
        }
        Ok(())
    }

    fn dump(&self) -> R<Value> {
        let mut names: Vec<Vec<u8>> = self.db.tree_names().iter().map(|n| n.to_vec()).collect();
        names.sort();
        let mut trees = Vec::new();
        for n in names {
            let name = String::from_utf8_lossy(&n).to_string();
            let t = self.tree(&name)?;
            let mut entries = Vec::new();
            for item in t.iter() {
                let (k, v) = item.map_err(|e| e.to_string())?;
                entries.push(json!([hex_encode(&k), hex_encode(&v)]));
            }
            trees.push(json!({"name": hex_encode(&n), "entries": entries}));
        }
        Ok(Value::Array(trees))
    }
}

// -------------------------------------------------------------- range helpers

fn range_bounds(step: &Map<String, Value>) -> R<(Bound<Vec<u8>>, Bound<Vec<u8>>)> {
    let lo = s_bytes(step, "lower")?;
    let hi = s_bytes(step, "upper")?;
    let lo_incl = s_bool(step, "lowerIncl", true);
    let hi_incl = s_bool(step, "upperIncl", false);
    let lb = match lo {
        None => Bound::Unbounded,
        Some(b) => {
            if lo_incl {
                Bound::Included(b)
            } else {
                Bound::Excluded(b)
            }
        }
    };
    let ub = match hi {
        None => Bound::Unbounded,
        Some(b) => {
            if hi_incl {
                Bound::Included(b)
            } else {
                Bound::Excluded(b)
            }
        }
    };
    Ok((lb, ub))
}

fn collect(iter: sled::Iter, reverse: bool) -> R<Value> {
    let mut out = Vec::new();
    if reverse {
        for item in iter.rev() {
            let (k, v) = item.map_err(|e: sled::Error| e.to_string())?;
            out.push(json!([hex_encode(&k), hex_encode(&v)]));
        }
    } else {
        for item in iter {
            let (k, v) = item.map_err(|e: sled::Error| e.to_string())?;
            out.push(json!([hex_encode(&k), hex_encode(&v)]));
        }
    }
    Ok(Value::Array(out))
}

// ------------------------------------------------------------------- one step

fn run_step(st: &mut State, step: &Map<String, Value>) -> R<Value> {
    let op = s_str(step, "op").ok_or("step missing op")?;
    let tname = s_str(step, "tree").unwrap_or_default();

    match op.as_str() {
        // ---- single-key writes and reads -------------------------------
        "insert" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let v = s_req_bytes(step, "value")?;
            let prev = t.insert(k, v).map_err(|e| e.to_string())?;
            Ok(opt_hex(prev.as_deref()))
        }
        "get" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let got = t.get(k).map_err(|e| e.to_string())?;
            Ok(opt_hex(got.as_deref()))
        }
        "remove" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let prev = t.remove(k).map_err(|e| e.to_string())?;
            Ok(opt_hex(prev.as_deref()))
        }
        "contains_key" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            Ok(json!(t.contains_key(k).map_err(|e| e.to_string())?))
        }
        "len" => Ok(json!(st.tree(&tname)?.len())),
        "is_empty" => Ok(json!(st.tree(&tname)?.is_empty())),
        "clear" => {
            st.tree(&tname)?.clear().map_err(|e| e.to_string())?;
            Ok(Value::Null)
        }

        // ---- compare_and_swap -----------------------------------------
        "cas" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let old = s_bytes(step, "old")?;
            let new = s_bytes(step, "new")?;
            let res = t
                .compare_and_swap(k, old, new)
                .map_err(|e| e.to_string())?;
            match res {
                Ok(()) => Ok(json!({"swapped": true, "current": Value::Null, "proposed": Value::Null})),
                Err(cas_err) => Ok(json!({
                    "swapped": false,
                    "current": opt_hex(cas_err.current.as_deref()),
                    "proposed": opt_hex(cas_err.proposed.as_deref()),
                })),
            }
        }

        // ---- read-modify-write ----------------------------------------
        "update_and_fetch" | "fetch_and_update" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let mode = s_str(step, "mut").unwrap_or_else(|| "concat".into());
            let arg = s_bytes(step, "value")?.unwrap_or_default();
            let f = move |old: Option<&[u8]>| merge_apply(&mode, b"", old, &arg);
            let out = if op == "update_and_fetch" {
                t.update_and_fetch(k, f).map_err(|e| e.to_string())?
            } else {
                t.fetch_and_update(k, f).map_err(|e| e.to_string())?
            };
            Ok(opt_hex(out.as_deref()))
        }

        // ---- batches ---------------------------------------------------
        "batch" => {
            let t = st.tree(&tname)?;
            let mut b = sled::Batch::default();
            let ops = step
                .get("ops")
                .and_then(|v| v.as_array())
                .ok_or("batch step missing ops")?;
            for sub in ops {
                let m = sub.as_object().ok_or("batch op must be an object")?;
                let sop = s_str(m, "op").ok_or("batch op missing op")?;
                let k = s_req_bytes(m, "key")?;
                match sop.as_str() {
                    "insert" => b.insert(k, s_req_bytes(m, "value")?),
                    "remove" => b.remove(k),
                    other => return Err(format!("unsupported batch op: {}", other)),
                }
            }
            t.apply_batch(b).map_err(|e| e.to_string())?;
            Ok(Value::Null)
        }

        // ---- iteration -------------------------------------------------
        "iter" => collect(st.tree(&tname)?.iter(), s_bool(step, "reverse", false)),
        "range" => {
            let t = st.tree(&tname)?;
            let b = range_bounds(step)?;
            collect(t.range::<Vec<u8>, _>(b), s_bool(step, "reverse", false))
        }
        "scan_prefix" => {
            let t = st.tree(&tname)?;
            let p = s_bytes(step, "prefix")?.unwrap_or_default();
            collect(t.scan_prefix(p), s_bool(step, "reverse", false))
        }

        // ---- ends and neighbours ---------------------------------------
        "first" => Ok(pair(st.tree(&tname)?.first().map_err(|e| e.to_string())?)),
        "last" => Ok(pair(st.tree(&tname)?.last().map_err(|e| e.to_string())?)),
        "get_gt" => {
            let k = s_req_bytes(step, "key")?;
            Ok(pair(
                st.tree(&tname)?.get_gt(k).map_err(|e| e.to_string())?,
            ))
        }
        "get_lt" => {
            let k = s_req_bytes(step, "key")?;
            Ok(pair(
                st.tree(&tname)?.get_lt(k).map_err(|e| e.to_string())?,
            ))
        }
        // sled 0.34.7 has no get_gte/get_lte; emulated over range() so the
        // Go-only helpers can still be verified against upstream semantics.
        "get_gte" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let mut it = t.range::<Vec<u8>, _>((Bound::Included(k), Bound::Unbounded));
            match it.next() {
                None => Ok(Value::Null),
                Some(r) => Ok(pair(Some(r.map_err(|e| e.to_string())?))),
            }
        }
        "get_lte" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let mut it = t
                .range::<Vec<u8>, _>((Bound::Unbounded, Bound::Included(k)))
                .rev();
            match it.next() {
                None => Ok(Value::Null),
                Some(r) => Ok(pair(Some(r.map_err(|e| e.to_string())?))),
            }
        }

        // ---- pop family -------------------------------------------------
        "pop_min" => Ok(pair(st.tree(&tname)?.pop_min().map_err(|e| e.to_string())?)),
        "pop_max" => Ok(pair(st.tree(&tname)?.pop_max().map_err(|e| e.to_string())?)),
        // sled 0.34.7 has no pop_first_in_range/pop_last_in_range; emulated as
        // range()+remove(), which is what those methods do.
        "pop_min_in_range" | "pop_max_in_range" => {
            let t = st.tree(&tname)?;
            let b = range_bounds(step)?;
            let found = if op == "pop_min_in_range" {
                t.range::<Vec<u8>, _>(b).next()
            } else {
                t.range::<Vec<u8>, _>(b).next_back()
            };
            match found {
                None => Ok(Value::Null),
                Some(r) => {
                    let (k, v) = r.map_err(|e| e.to_string())?;
                    t.remove(&k).map_err(|e| e.to_string())?;
                    Ok(json!([hex_encode(&k), hex_encode(&v)]))
                }
            }
        }

        // ---- trees ------------------------------------------------------
        "open_tree" => {
            let name = s_str(step, "name").ok_or("open_tree missing name")?;
            if name.is_empty() {
                return Err("empty tree name".into());
            }
            let t = st.db.open_tree(name.as_bytes()).map_err(|e| e.to_string())?;
            Ok(Value::String(hex_encode(&t.name())))
        }
        "drop_tree" => {
            let name = s_str(step, "name").ok_or("drop_tree missing name")?;
            Ok(json!(st
                .db
                .drop_tree(name.as_bytes())
                .map_err(|e| e.to_string())?))
        }
        "tree_names" => {
            let mut names: Vec<String> = st
                .db
                .tree_names()
                .iter()
                .map(|n| hex_encode(n))
                .collect();
            names.sort();
            Ok(json!(names))
        }
        "tree_name" => Ok(Value::String(hex_encode(&st.tree(&tname)?.name()))),

        // ---- merge operators --------------------------------------------
        "set_merge_operator" => {
            let which = s_str(step, "merge").ok_or("set_merge_operator missing merge")?;
            st.install_merge(&tname, &which)?;
            st.merge_ops.insert(tname.clone(), which);
            Ok(Value::Null)
        }
        "merge" => {
            let t = st.tree(&tname)?;
            let k = s_req_bytes(step, "key")?;
            let v = s_req_bytes(step, "value")?;
            let out = t.merge(k, v).map_err(|e| e.to_string())?;
            Ok(opt_hex(out.as_deref()))
        }

        // ---- ids, durability, lifecycle ----------------------------------
        "generate_id_monotonic" => {
            let n = s_u64(step, "n", 5);
            let mut prev = st.last_id;
            for _ in 0..n {
                let id = st.db.generate_id().map_err(|e| e.to_string())?;
                if let Some(p) = prev {
                    if id <= p {
                        return Ok(json!(false));
                    }
                }
                prev = Some(id);
            }
            st.last_id = prev;
            Ok(json!(true))
        }
        "flush" => {
            st.db.flush().map_err(|e| e.to_string())?;
            Ok(Value::Null)
        }
        "was_recovered" => Ok(json!(st.db.was_recovered())),
        "reopen" => {
            st.reopen()?;
            Ok(Value::Null)
        }
        // Upstream sled 0.34.7 has no explicit compaction entry point: the log
        // is GC'd continuously. Modelled here as flush, i.e. a no-op that must
        // preserve every tree and key. The Go port's Compact() must do the same.
        "compact" => {
            st.db.flush().map_err(|e| e.to_string())?;
            Ok(Value::Null)
        }
        "dump" => st.dump(),

        other => Err(format!("unsupported op: {}", other)),
    }
}

// ------------------------------------------------------------------- one case

fn tmp_dir(seq: u64) -> PathBuf {
    std::env::temp_dir().join(format!("sled-parity-rs-{}-{}", std::process::id(), seq))
}

fn run_case(steps: &[Value], seq: u64) -> R<Value> {
    let dir = tmp_dir(seq);
    let _ = std::fs::remove_dir_all(&dir);
    let mut st = State::open(dir.clone())?;
    let mut out = Vec::with_capacity(steps.len());
    for s in steps {
        let m = match s.as_object() {
            Some(m) => m,
            None => {
                out.push(json!({"error": true}));
                continue;
            }
        };
        match run_step(&mut st, m) {
            Ok(v) => out.push(v),
            Err(_) => out.push(json!({"error": true})),
        }
    }
    let trees = st.dump()?;
    drop(st);
    let _ = std::fs::remove_dir_all(&dir);
    Ok(json!({"steps": out, "trees": trees}))
}

fn main() {
    let stdin = io::stdin();
    let mut stdout = io::stdout();
    let mut seq: u64 = 0;
    for line in stdin.lock().lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };
        if line.trim().is_empty() {
            continue;
        }
        seq += 1;
        let parsed: Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(e) => {
                let _ = writeln!(stdout, "{}", json!({"id": "?", "ok": false, "error": e.to_string()}));
                let _ = stdout.flush();
                continue;
            }
        };
        let id = parsed
            .get("id")
            .and_then(|v| v.as_str())
            .unwrap_or("?")
            .to_string();
        let steps: Vec<Value> = parsed
            .get("args")
            .and_then(|v| v.as_array())
            .and_then(|a| a.first())
            .and_then(|v| v.as_array())
            .cloned()
            .unwrap_or_default();

        let seq_copy = seq;
        let res = std::panic::catch_unwind(move || run_case(&steps, seq_copy));
        let out = match res {
            Ok(Ok(v)) => json!({"id": id, "ok": true, "value": v}),
            Ok(Err(e)) => json!({"id": id, "ok": false, "error": e}),
            Err(_) => json!({"id": id, "ok": false, "error": "panic"}),
        };
        let _ = writeln!(stdout, "{}", out);
        let _ = stdout.flush();
    }
}
