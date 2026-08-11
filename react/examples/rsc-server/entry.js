// The browser half of the bridge.
//
// This is the file `react rsc` generates. It is checked in here so the example
// runs with no toolchain at all.

import { createFromFetch } from "react-server-dom-webpack/client.browser";
import { createRoot } from "react-dom/client";

// React resolves a client reference by awaiting each chunk and then requiring
// the module id. With no bundler involved both of those are simply the module's
// URL, so these two functions are the entire module system: import it, remember
// it, hand it back. A bundler would normally supply them.
const loaded = new Map();

globalThis.__webpack_chunk_load__ = async (url) => {
  if (!loaded.has(url)) {
    loaded.set(url, await import(url));
  }
  return loaded.get(url);
};

globalThis.__webpack_require__ = (url) => loaded.get(url);

const root = createRoot(document.getElementById("root"));

createFromFetch(fetch("/rsc")).then(
  (tree) => root.render(tree),
  (error) => {
    console.error("react: could not load the server payload", error);
    throw error;
  }
);
