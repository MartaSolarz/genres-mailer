(function () {
    "use strict";

    var csrf = document.body.getAttribute("data-csrf");
    var currentUser = document.body.getAttribute("data-user");
    var DROPZONE_DEFAULT = "Przeciągnij plik PDF tutaj lub kliknij, aby wybrać";

    var sample = document.getElementById("sample");
    var dropzone = document.getElementById("dropzone");
    var dropzoneText = document.getElementById("dropzoneText");
    var fileInput = document.getElementById("fileInput");
    var processBtn = document.getElementById("processBtn");
    var resetBtn = document.getElementById("resetBtn");
    var statusEl = document.getElementById("status");
    var historyBody = document.getElementById("historyBody");

    var result = document.getElementById("result");
    var resTitle = document.getElementById("resTitle");
    var resSample = document.getElementById("resSample");
    var resRecipient = document.getElementById("resRecipient");
    var previewBtn = document.getElementById("previewBtn");
    var downloadBtn = document.getElementById("downloadBtn");
    var passwordBtn = document.getElementById("passwordBtn");

    var modal = document.getElementById("passwordModal");
    var pwValue = document.getElementById("pwValue");
    var copyPwBtn = document.getElementById("copyPwBtn");
    var closePwBtn = document.getElementById("closePwBtn");

    var selectedFile = null;
    var currentJob = null;

    function refreshProcessState() {
        processBtn.disabled = !(sample.value && selectedFile);
    }

    function setStatus(msg, isError) {
        statusEl.textContent = msg || "";
        statusEl.classList.toggle("status-error", !!isError);
    }

    function chooseFile(file) {
        if (!file) {
            return;
        }

        var isPdf = file.type === "application/pdf" || /\.pdf$/i.test(file.name);
        if (!isPdf) {
            setStatus("Dozwolone są wyłącznie pliki PDF.", true);
            return;
        }

        selectedFile = file;
        dropzoneText.textContent = file.name;
        setStatus("");
        refreshProcessState();
    }

    function resetForm() {
        sample.value = "";
        selectedFile = null;
        currentJob = null;
        fileInput.value = "";
        dropzoneText.textContent = DROPZONE_DEFAULT;
        result.classList.add("hidden");
        setStatus("");
        acClose();
        refreshProcessState();
    }

    function pad(n) {
        return (n < 10 ? "0" : "") + n;
    }

    function nowStamp() {
        var d = new Date();
        return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) +
            " " + pad(d.getHours()) + ":" + pad(d.getMinutes());
    }

    function prependHistory(uuid, sampleID, recipient) {
        var empty = document.getElementById("historyEmpty");
        if (empty) {
            empty.remove();
        }

        var tr = document.createElement("tr");
        tr.setAttribute("data-uuid", uuid);
        tr.setAttribute("data-sample", sampleID);
        tr.setAttribute("data-recipient", recipient);
        tr.setAttribute("data-status", "encrypted");

        var tdSample = document.createElement("td");
        tdSample.textContent = sampleID;

        var tdUser = document.createElement("td");
        tdUser.textContent = currentUser;

        var tdStatus = document.createElement("td");
        var badge = document.createElement("span");
        badge.className = "badge badge-encrypted";
        badge.textContent = "Zaszyfrowany";
        tdStatus.appendChild(badge);

        var tdDate = document.createElement("td");
        tdDate.className = "nowrap";
        tdDate.textContent = nowStamp();

        var tdAction = document.createElement("td");
        tdAction.className = "cell-action";

        var openBtn = document.createElement("button");
        openBtn.type = "button";
        openBtn.className = "btn-ghost btn-xs js-open";
        openBtn.textContent = "Otwórz";

        var delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "btn-ghost btn-xs btn-danger js-delete";
        delBtn.textContent = "Usuń";

        tdAction.appendChild(openBtn);
        tdAction.appendChild(delBtn);

        tr.appendChild(tdSample);
        tr.appendChild(tdUser);
        tr.appendChild(tdStatus);
        tr.appendChild(tdDate);
        tr.appendChild(tdAction);
        historyBody.insertBefore(tr, historyBody.firstChild);
    }

    function openJob(uuid, sampleID, recipient, title) {
        currentJob = uuid;
        resTitle.textContent = title;
        resSample.textContent = sampleID;
        resRecipient.textContent = recipient || "—";
        result.classList.remove("hidden");
        result.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }

    var optionsList = document.getElementById("sampleOptions");
    var samples = [];
    var acItems = [];
    var acActive = -1;

    (function loadSamples() {
        var tpl = document.getElementById("sampleData");
        if (!tpl || !tpl.content) {
            return;
        }

        Array.prototype.forEach.call(tpl.content.querySelectorAll("span"), function (s) {
            samples.push({ id: s.getAttribute("data-id"), masked: s.getAttribute("data-masked") });
        });
    })();

    function acClose() {
        optionsList.classList.add("hidden");
        optionsList.innerHTML = "";
        sample.setAttribute("aria-expanded", "false");
        acItems = [];
        acActive = -1;
    }

    function acRender(query) {
        var q = query.trim().toLowerCase();

        acItems = samples.filter(function (s) {
            return !q || s.id.toLowerCase().indexOf(q) !== -1 || s.masked.toLowerCase().indexOf(q) !== -1;
        }).slice(0, 8);

        if (!acItems.length) {
            acClose();
            return;
        }

        optionsList.innerHTML = "";

        acItems.forEach(function (s) {
            var li = document.createElement("li");
            li.className = "autocomplete-item";
            li.setAttribute("role", "option");
            li.setAttribute("data-id", s.id);

            var idSpan = document.createElement("span");
            idSpan.className = "ac-id";
            idSpan.textContent = s.id;

            var maskSpan = document.createElement("span");
            maskSpan.className = "ac-mask";
            maskSpan.textContent = s.masked;

            li.appendChild(idSpan);
            li.appendChild(maskSpan);
            optionsList.appendChild(li);
        });

        optionsList.classList.remove("hidden");
        sample.setAttribute("aria-expanded", "true");
        acActive = -1;
    }

    function acSetActive(i) {
        var lis = optionsList.querySelectorAll(".autocomplete-item");
        Array.prototype.forEach.call(lis, function (li) {
            li.classList.remove("active");
        });

        if (i >= 0 && i < lis.length) {
            lis[i].classList.add("active");
            lis[i].scrollIntoView({ block: "nearest" });
        }

        acActive = i;
    }

    function acSelect(id) {
        sample.value = id;
        acClose();
        refreshProcessState();
        sample.focus();
    }

    sample.addEventListener("input", function () {
        refreshProcessState();
        acRender(sample.value);
    });

    sample.addEventListener("focus", function () {
        acRender(sample.value);
    });

    sample.addEventListener("keydown", function (e) {
        if (optionsList.classList.contains("hidden")) {
            if (e.key === "ArrowDown") {
                acRender(sample.value);
            }
            return;
        }

        if (e.key === "ArrowDown") {
            e.preventDefault();
            acSetActive(Math.min(acActive + 1, acItems.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            acSetActive(Math.max(acActive - 1, 0));
        } else if (e.key === "Enter" && acActive >= 0) {
            e.preventDefault();
            acSelect(acItems[acActive].id);
        } else if (e.key === "Escape") {
            acClose();
        }
    });

    optionsList.addEventListener("mousedown", function (e) {
        var li = e.target.closest(".autocomplete-item");
        if (li) {
            e.preventDefault();
            acSelect(li.getAttribute("data-id"));
        }
    });

    document.addEventListener("click", function (e) {
        if (!e.target.closest(".autocomplete")) {
            acClose();
        }
    });

    resetBtn.addEventListener("click", resetForm);

    historyBody.addEventListener("click", function (e) {
        var openBtn = e.target.closest(".js-open");
        if (openBtn) {
            var row = openBtn.closest("tr");
            openJob(row.getAttribute("data-uuid"), row.getAttribute("data-sample"),
                row.getAttribute("data-recipient"), "Dokument");
            return;
        }

        var delBtn = e.target.closest(".js-delete");
        if (delBtn) {
            handleDelete(delBtn);
        }
    });

    function handleDelete(btn) {
        if (btn.getAttribute("data-confirm") !== "1") {
            btn.setAttribute("data-confirm", "1");
            btn.textContent = "Na pewno?";
            setTimeout(function () {
                if (btn.getAttribute("data-confirm") === "1") {
                    btn.setAttribute("data-confirm", "0");
                    btn.textContent = "Usuń";
                }
            }, 3000);
            return;
        }

        var row = btn.closest("tr");
        var uuid = row.getAttribute("data-uuid");
        btn.disabled = true;

        fetch("/api/jobs/" + uuid, {
            method: "DELETE",
            headers: { "X-CSRF-Token": csrf }
        }).then(function (resp) {
            if (!resp.ok) {
                btn.disabled = false;
                btn.setAttribute("data-confirm", "0");
                btn.textContent = "Usuń";
                return resp.json().then(function (data) {
                    setStatus(data.error || "Nie udało się usunąć dokumentu.", true);
                });
            }

            if (currentJob === uuid) {
                resetForm();
            }

            row.remove();

            if (!historyBody.querySelector("tr")) {
                var empty = document.createElement("tr");
                empty.id = "historyEmpty";
                empty.innerHTML = '<td colspan="5" class="muted">Brak dokumentów.</td>';
                historyBody.appendChild(empty);
            }
        }).catch(function () {
            btn.disabled = false;
            setStatus("Błąd połączenia z serwerem.", true);
        });
    }

    dropzone.addEventListener("click", function () {
        fileInput.click();
    });

    dropzone.addEventListener("keydown", function (e) {
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            fileInput.click();
        }
    });

    fileInput.addEventListener("change", function () {
        chooseFile(fileInput.files[0]);
    });

    ["dragenter", "dragover"].forEach(function (evt) {
        dropzone.addEventListener(evt, function (e) {
            e.preventDefault();
            dropzone.classList.add("dragover");
        });
    });

    ["dragleave", "drop"].forEach(function (evt) {
        dropzone.addEventListener(evt, function (e) {
            e.preventDefault();
            dropzone.classList.remove("dragover");
        });
    });

    dropzone.addEventListener("drop", function (e) {
        if (e.dataTransfer && e.dataTransfer.files.length) {
            chooseFile(e.dataTransfer.files[0]);
        }
    });

    processBtn.addEventListener("click", function () {
        if (!sample.value || !selectedFile) {
            return;
        }

        processBtn.disabled = true;
        setStatus("Przetwarzanie…");

        var form = new FormData();
        form.append("sample_id", sample.value);
        form.append("file", selectedFile);

        fetch("/api/jobs", {
            method: "POST",
            headers: { "X-CSRF-Token": csrf },
            body: form
        }).then(function (resp) {
            return resp.json().then(function (data) {
                return { ok: resp.ok, data: data };
            });
        }).then(function (r) {
            if (!r.ok) {
                setStatus(r.data.error || "Błąd przetwarzania.", true);
                refreshProcessState();
                return;
            }

            currentJob = r.data.job_uuid;
            resTitle.textContent = "Dokument gotowy";
            resSample.textContent = r.data.sample_id;
            resRecipient.textContent = r.data.recipient_masked;
            result.classList.remove("hidden");
            prependHistory(r.data.job_uuid, r.data.sample_id, r.data.recipient_masked);
            setStatus("Dokument został zaszyfrowany.");
        }).catch(function () {
            setStatus("Błąd połączenia z serwerem.", true);
            refreshProcessState();
        });
    });

    previewBtn.addEventListener("click", function () {
        if (!currentJob) {
            return;
        }

        window.open("/api/jobs/" + currentJob + "/preview", "_blank", "noopener");
    });

    downloadBtn.addEventListener("click", function () {
        if (!currentJob) {
            return;
        }

        window.location.href = "/api/jobs/" + currentJob + "/download";
    });

    passwordBtn.addEventListener("click", function () {
        if (!currentJob) {
            return;
        }

        fetch("/api/jobs/" + currentJob + "/password").then(function (resp) {
            return resp.json().then(function (data) {
                return { ok: resp.ok, data: data };
            });
        }).then(function (r) {
            if (!r.ok) {
                setStatus(r.data.error || "Nie udało się pobrać hasła.", true);
                return;
            }

            pwValue.textContent = r.data.password;
            modal.classList.remove("hidden");
        }).catch(function () {
            setStatus("Błąd połączenia z serwerem.", true);
        });
    });

    copyPwBtn.addEventListener("click", function () {
        if (navigator.clipboard) {
            navigator.clipboard.writeText(pwValue.textContent).then(function () {
                copyPwBtn.textContent = "Skopiowano";
            });
        }
    });

    closePwBtn.addEventListener("click", function () {
        modal.classList.add("hidden");
        pwValue.textContent = "";
        copyPwBtn.textContent = "Kopiuj";
    });
})();
