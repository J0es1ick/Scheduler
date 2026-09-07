let loggedOut = false;
export function explicitlyLoggedOut() {
  try {
    return (
      loggedOut || sessionStorage.getItem("scheduler:explicit-logout") === "1"
    );
  } catch {
    return loggedOut;
  }
}
export function setExplicitLogout(value: boolean) {
  loggedOut = value;
  try {
    if (value) sessionStorage.setItem("scheduler:explicit-logout", "1");
    else sessionStorage.removeItem("scheduler:explicit-logout");
  } catch {
    return;
  }
}
