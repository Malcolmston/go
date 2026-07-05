// TypeScript schema for the Go API-documentation JSON consumed by the shared
// docs renderer. It mirrors the model of the standard library's go/doc package
// (as used by the static gendocs generator): a module is a set of packages, and
// each package carries its doc plus exported consts, vars, types, and funcs.
// Generators in other repos emit this shape from go/doc; the React components in
// components/docs render it.

// DocValue is a single `const`/`var` declaration group. go/doc groups related
// identifiers (e.g. an `const ( … )` enum block) into one *doc.Value, so `names`
// may contain several identifiers and `signature` is the whole declaration.
export interface DocValue {
  names: string[];
  signature: string;
  doc: string;
}

// DocFunc is an exported function or a method. `recv` is set for methods and
// holds the receiver type as written (e.g. "*Router"); it is empty/undefined for
// package-level funcs and type constructors.
export interface DocFunc {
  name: string;
  recv?: string;
  signature: string;
  doc: string;
}

// DocExample is a runnable go/doc example. `name` is the suffix after the
// underscore ("" for a package-level example); `output` is the expected output
// comment if the example declared one.
export interface DocExample {
  name: string;
  code: string;
  output?: string;
  doc?: string;
}

// DocType is an exported type together with everything go/doc scopes under it:
// the type declaration, its type-scoped consts/vars (e.g. an enum), constructor
// funcs that return it, and its methods.
export interface DocType {
  name: string;
  signature: string;
  doc: string;
  consts: DocValue[];
  vars: DocValue[];
  funcs: DocFunc[];
  methods: DocFunc[];
  examples?: DocExample[];
}

// DocPackage is one importable package. `synopsis` is the first sentence of the
// package doc (doc.Synopsis); `isCommand` marks a `package main` command.
export interface DocPackage {
  importPath: string;
  name: string;
  synopsis: string;
  doc: string;
  isCommand?: boolean;
  consts: DocValue[];
  vars: DocValue[];
  types: DocType[];
  funcs: DocFunc[];
  examples?: DocExample[];
}

// DocIndex is the top-level document: one module and all of its packages.
export interface DocIndex {
  module: string;
  generatedAt?: string;
  packages: DocPackage[];
}
