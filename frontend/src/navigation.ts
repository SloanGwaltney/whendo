// The app's main pages, in the order the sidebar lists them. See
// docs/design/mvp-ui.md#navigation.
export type PageId = "home" | "redeems" | "runHistory" | "settings";

export interface NavigationItem {
  pageId: PageId;
  label: string;
}

export const navigationItems: NavigationItem[] = [
  {pageId: "home", label: "Home"},
  {pageId: "redeems", label: "Redeems"},
  {pageId: "runHistory", label: "Run History"},
  {pageId: "settings", label: "Settings"},
];

export const startPageId: PageId = "home";
