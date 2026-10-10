// The files of a config archive as a folder tree, for choosing which go in or
// out (#198). Paths are the ones the installer lists - as tar stores them, from
// the root of the image - so what is chosen goes back to it unchanged.

// What every path starts with and nobody needs to see: the printer's home and
// its data folder. Left out of the tree, so it opens at config/, database/ and
// so on.
const HIDDEN = ["home", "home/printer", "home/printer/printer_data"];

export function buildTree(files) {
  const root = { name: "", path: "", children: new Map(), files: [] };
  for (const file of files) {
    const parts = file.split("/").filter(Boolean);
    let node = root;
    let path = "";
    parts.forEach((part, i) => {
      path = path ? path + "/" + part : part;
      if (!node.children.has(part)) {
        node.children.set(part, { name: part, path, children: new Map(), files: [], leaf: i === parts.length - 1 });
      }
      node = node.children.get(part);
    });
  }
  const count = (n) => {
    n.files = n.leaf ? [n.path] : [].concat(...[...n.children.values()].map((c) => count(c)));
    return n.files;
  };
  count(root);
  return root;
}

// The nodes to show at the top: the children of the hidden prefix folders.
export function visibleRoots(root) {
  let nodes = [...root.children.values()];
  for (;;) {
    const hidden = nodes.filter((n) => HIDDEN.includes(n.path));
    if (!hidden.length) break;
    nodes = nodes.filter((n) => !HIDDEN.includes(n.path)).concat(...hidden.map((n) => [...n.children.values()]));
  }
  return nodes.sort((a, b) => Number(!!a.children.size === false) - Number(!!b.children.size === false) || a.name.localeCompare(b.name));
}

// The --include list for a choice of files: a folder whole when everything in
// it is chosen, otherwise its files. [] means everything (so a backup is what
// it always was); null means nothing, which is no backup.
export function includeFor(files, selected) {
  const chosen = new Set(selected);
  if (!files.length || files.every((f) => chosen.has(f))) return [];
  if (!files.some((f) => chosen.has(f))) return null;
  const out = [];
  const walk = (node) => {
    if (!node.files.some((f) => chosen.has(f))) return;
    if (node.files.every((f) => chosen.has(f)) && node.path) {
      out.push(node.path);
      return;
    }
    [...node.children.values()].forEach(walk);
  };
  walk(buildTree(files));
  return out;
}

// What the Klipper preset ticks: the files Klipper itself reads, which is the
// printer's config and the firmware options, not Moonraker or the UI.
export function isKlipperFile(path) {
  const p = path.replace(/^home\/printer\/printer_data\//, "");
  if (!p.startsWith("config/")) return false;
  const name = p.split("/").pop();
  if (/^(moonraker\.conf|KlipperScreen\.conf|\.moonraker\.conf\.bkp)$/.test(name)) return false;
  if (/^(fluidd|mainsail)\.cfg$/.test(name)) return false;
  return /\.(cfg|config)$/.test(name);
}
