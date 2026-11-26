// Applied before first paint so there is no flash of the wrong theme.
// Rover is dark first: dark unless the user picked light (or "system").
(function () {
  var t = null;
  try { t = localStorage.getItem("hp-theme"); } catch (e) {}
  if (t === "system") t = window.matchMedia && matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  if (t !== "light" && t !== "dark") t = "dark";
  document.documentElement.setAttribute("data-theme", t);
})();
