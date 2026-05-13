// ==========================================
// GLOBAL STATE & ERROR HANDLING
// ==========================================
let currentVault = null;
let generatedRequestToken = "";

window.onerror = function(msg, url, line) {
    const errBox = document.getElementById('fatal-error-log');
    errBox.innerText = `FATAL JS ERROR: ${msg} (Line ${line})`;
    errBox.classList.remove('hidden');
};

window.onload = () => {
    setTimeout(async () => {
        if (window.runtime && window.runtime.EventsOn) {
            window.runtime.EventsOn("vpn-state-change", (newState) => {
                syncToggleUI(newState);
            });
        }
        await refreshState();
    }, 100);
};

// ==========================================
// STATE MANAGEMENT & NAVIGATION
// ==========================================
async function refreshState() {
    try {
        currentVault = await window.go.main.App.GetVault();
        
        // 🎯 Check if we have any profiles at all
        const profiles = currentVault.profiles || {};
        const activeID = currentVault.active_profile_id || "";
        const activeProfile = profiles[activeID];

        // If no servers exist, or if we are enrolling a new one
        if (!activeProfile || Object.keys(profiles).length === 0) {
            showScreen('view-enroll');
            document.getElementById('btn-cancel-enroll').classList.add('hidden');
            return;
        }

        // Enrollment handling for a pending profile
        if (activeProfile.status === "pending_approval") {
            showScreen('view-enroll');
            document.getElementById('enroll-title').innerText = "Enrolling Node";
            document.getElementById('enroll-subtitle').innerText = "Follow the steps to approve this device.";
            
            // Show cancel button only if they have other servers they could go back to
            if (Object.keys(profiles).length > 1) {
                document.getElementById('btn-cancel-enroll').classList.remove('hidden');
            }
            return;
        }

        // Active Dashboard logic
        if (activeProfile.status === "active") {
            document.getElementById('dash-ip').innerText = activeProfile.assigned_ip;
            document.getElementById('dash-endpoint').innerText = activeProfile.server_endpoint;
            document.getElementById('toggle-global-route').checked = currentVault.use_global_routing;
            
            updateProfileDropdown(profiles, activeID);
            showScreen('view-dashboard');
            await syncToggleUI();
        }
    } catch (err) {
        console.error("State Refresh Failed:", err);
    }
}

// 🎯 NEW: Populate the server list dropdown
function updateProfileDropdown(profiles, activeID) {
    const selector = document.getElementById('profile-selector');
    selector.innerHTML = '';
    
    Object.values(profiles).forEach(profile => {
        if (profile.status === "active") {
            const opt = document.createElement('option');
            opt.value = profile.id;
            opt.innerText = profile.name || `Server ${profile.assigned_ip}`;
            if (profile.id === activeID) opt.selected = true;
            selector.appendChild(opt);
        }
    });
}

function showScreen(screenId) {
    document.getElementById('view-enroll').classList.add('hidden');
    document.getElementById('view-dashboard').classList.add('hidden');
    const target = document.getElementById(screenId);
    target.classList.remove('hidden');
    target.classList.add('animate-fade-in');
}

// ==========================================
// MULTI-SERVER MANAGEMENT
// ==========================================

async function handleProfileChange(profileID) {
    if (!profileID) return;
    try {
        await window.go.main.App.SetActiveProfile(profileID);
        await refreshState();
    } catch (err) {
        alert("Failed to switch server: " + err);
    }
}

function showEnrollmentForNewServer() {
    // Reset enrollment UI
    generatedRequestToken = "";
    document.getElementById('req-copy-container').classList.add('hidden');
    document.getElementById('btn-generate-req').innerText = "Generate Request Token";
    document.getElementById('input-visa').value = "";
    
    showScreen('view-enroll');
    document.getElementById('btn-cancel-enroll').classList.remove('hidden');
}

async function cancelEnrollment() {
    // If they cancel, we need to find the first 'active' profile and switch back to it
    const profiles = currentVault.profiles || {};
    const firstActive = Object.values(profiles).find(p => p.status === "active");
    
    if (firstActive) {
        await window.go.main.App.SetActiveProfile(firstActive.id);
    }
    await refreshState();
}

async function handleRenameServer() {
    const activeID = currentVault.active_profile_id;
    const currentName = currentVault.profiles[activeID].name;
    const newName = prompt("Enter a new name for this server:", currentName);
    
    if (newName && newName !== currentName) {
        await window.go.main.App.RenameProfile(activeID, newName);
        await refreshState();
    }
}

async function handleRemoveServer() {
    const activeID = currentVault.active_profile_id;
    if (!confirm("Are you sure you want to delete this server profile?")) return;

    try {
        await window.go.main.App.RemoveProfile(activeID);
        await refreshState();
    } catch (err) {
        alert("Error removing server: " + err);
    }
}

// ==========================================
// ENROLLMENT & DASHBOARD LOGIC (Updated)
// ==========================================

async function handleGenerateRequest() {
    const btn = document.getElementById('btn-generate-req');
    btn.innerText = "Generating...";
    btn.disabled = true;

    try {
        generatedRequestToken = await window.go.main.App.GenerateRequest();
        document.getElementById('req-copy-container').classList.remove('hidden');
        btn.innerText = "Regenerate Token";
    } catch (err) {
        alert("Error: " + err);
    } finally {
        btn.disabled = false;
    }
}

function copyClientRequest() {
    if (!generatedRequestToken) return;
    navigator.clipboard.writeText(generatedRequestToken).then(() => {
        const copyBtn = document.getElementById('btn-copy-req');
        const originalHTML = copyBtn.innerHTML;
        copyBtn.innerHTML = `<span>✔ Copied!</span>`;
        setTimeout(() => { copyBtn.innerHTML = originalHTML; }, 2000);
    });
}

async function handleVerify() {
    const tokenInput = document.getElementById('input-visa').value.trim();
    if (!tokenInput) return;

    try {
        await window.go.main.App.ProcessApproval(tokenInput);
        await refreshState();
    } catch (err) {
        alert("Verification failed: " + err);
    }
}

async function syncToggleUI(forcedState = null) {
    const state = forcedState || await window.go.main.App.GetTunnelState();
    const routeToggle = document.getElementById('toggle-global-route');
    const routeWarning = document.getElementById('route-warning');
    const btnText = document.getElementById('toggle-text');
    const btnIndicator = document.getElementById('btn-status-indicator');
    const shieldTitle = document.getElementById('status-title');

    // UI Locking
    const isDisconnected = (state === "disconnected");
    routeToggle.disabled = !isDisconnected;
    document.getElementById('profile-selector').disabled = !isDisconnected;
    routeWarning.classList.toggle('hidden', isDisconnected);

    // Color logic
    btnIndicator.className = "w-3 h-3 rounded-full transition-all duration-300";
    if (state === "secured") {
        btnText.innerText = "Disconnect Tunnel";
        btnIndicator.classList.add('bg-success');
        shieldTitle.innerText = "Secured";
    } else if (state === "disconnected") {
        btnText.innerText = "Connect Tunnel";
        btnIndicator.classList.add('bg-slate-500');
        shieldTitle.innerText = "Disconnected";
    } else {
        btnText.innerText = "Pending...";
        btnIndicator.classList.add('bg-yellow-400', 'animate-pulse');
        shieldTitle.innerText = state.toUpperCase();
    }
}

async function handleToggleVPN() {
    try {
        const state = await window.go.main.App.GetTunnelState();
        if (state === "disconnected") await window.go.main.App.Connect();
        else await window.go.main.App.Disconnect();
    } catch (err) {
        alert("VPN Error: " + err);
    }
}

async function handleToggleRouting() {
    const isEnabled = document.getElementById('toggle-global-route').checked;
    await window.go.main.App.ToggleGlobalRouting(isEnabled);
}