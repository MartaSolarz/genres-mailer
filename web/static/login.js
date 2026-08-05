(function () {
    "use strict";

    var input = document.getElementById("password");
    var btn = document.getElementById("togglePassword");

    if (!input || !btn) {
        return;
    }

    btn.addEventListener("click", function () {
        var show = input.type === "password";
        input.type = show ? "text" : "password";
        btn.classList.toggle("revealed", show);
        btn.setAttribute("aria-label", show ? "Ukryj hasło" : "Pokaż hasło");
        btn.setAttribute("aria-pressed", show ? "true" : "false");
        input.focus();
    });
})();
