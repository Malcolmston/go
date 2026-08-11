// app/api/graphql/depthLimit.ts — query-depth validation rule for /api/graphql.
//
// This lives outside route.ts because Next.js only permits a fixed set of
// exports from a Route Handler file (the HTTP-method handlers plus route-segment
// config); exporting a helper from route.ts fails `next build`. Keeping it here
// also makes the rule directly unit-testable.
//
// Why it exists: Package.imports / importedBy / related all return Packages, so
// the schema is cyclic — `{ package { imports { importedBy { imports … } } } }`
// nests arbitrarily deep, and every level re-scans the whole edge list. A short
// query string can therefore cost unbounded CPU. This rule rejects any operation
// nested deeper than `maxDepth` during validation, before a resolver ever runs.
//
// Depth counts *field* levels. Inline fragments and fragment spreads are
// traversed transparently (they contribute no depth of their own) so nesting
// cannot be hidden behind a fragment. Fragment cycles are separately reported by
// NoFragmentCyclesRule; the `seen` set here keeps the walk terminating anyway.

import { GraphQLError, Kind } from 'graphql';
import type {
  FragmentDefinitionNode,
  SelectionSetNode,
  ValidationContext,
  ValidationRule,
} from 'graphql';

/** Default maximum nesting depth for a single operation. */
export const MAX_QUERY_DEPTH = 10;

/** Compute the field-nesting depth of a selection set, following fragments. */
function depthOf(
  selectionSet: SelectionSetNode,
  fragments: Map<string, FragmentDefinitionNode>,
  seen: Set<string>,
): number {
  let deepest = 0;
  for (const sel of selectionSet.selections) {
    if (sel.kind === Kind.FIELD) {
      const child = sel.selectionSet ? depthOf(sel.selectionSet, fragments, seen) : 0;
      deepest = Math.max(deepest, 1 + child);
    } else if (sel.kind === Kind.INLINE_FRAGMENT) {
      deepest = Math.max(deepest, depthOf(sel.selectionSet, fragments, seen));
    } else if (sel.kind === Kind.FRAGMENT_SPREAD) {
      const name = sel.name.value;
      if (seen.has(name)) continue; // cyclic spread — reported by another rule
      const frag = fragments.get(name);
      if (!frag) continue; // unknown fragment — reported by another rule
      seen.add(name);
      deepest = Math.max(deepest, depthOf(frag.selectionSet, fragments, seen));
      seen.delete(name);
    }
  }
  return deepest;
}

/** Build a graphql-js ValidationRule that rejects operations deeper than maxDepth. */
export function depthLimitRule(maxDepth: number = MAX_QUERY_DEPTH): ValidationRule {
  return (context: ValidationContext) => {
    const fragments = new Map<string, FragmentDefinitionNode>();
    for (const def of context.getDocument().definitions) {
      if (def.kind === Kind.FRAGMENT_DEFINITION) fragments.set(def.name.value, def);
    }

    return {
      OperationDefinition(node) {
        const depth = depthOf(node.selectionSet, fragments, new Set());
        if (depth > maxDepth) {
          context.reportError(
            new GraphQLError(
              `Query is too deep: ${depth} exceeds the maximum depth of ${maxDepth}.`,
              { nodes: [node] },
            ),
          );
        }
      },
    };
  };
}
