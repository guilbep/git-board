// Links built from forge data (an issue's forges[].url, a conflict's url, a
// pull request's): that data travels through git, so anyone who can push the
// board's refs can write it. Only http(s) URLs become links: a javascript:
// or data: URL would run in the app's origin, with the whole local API.

// safeHref is url when it is an absolute http(s) URL, else undefined (Vue
// then leaves the href out, and the link goes nowhere).
export function safeHref(url: string | null | undefined): string | undefined {
  if (!url) return undefined;
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return undefined; // relative, or not a URL
  }
  return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : undefined;
}

// forgeBase is where an issue's #123 and @name go, from the URL of the
// forge issue it came from: that URL without its number (null when it does
// not end with one), and the forge's origin. null when the URL is not a
// safe one.
export function forgeBase(url: string | null | undefined): { refs: string | null; people: string } | null {
  const href = safeHref(url);
  if (!href) return null;
  const u = new URL(href);
  const numbered = /\d+$/.test(u.pathname) && !u.search && !u.hash;
  return { refs: numbered ? href.replace(/\d+$/, '') : null, people: `${u.origin}/` };
}
