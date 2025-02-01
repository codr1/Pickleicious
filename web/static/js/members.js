// web/static/js/members.js

document.addEventListener('htmx:afterRequest', function(evt) {
    console.log("HTMX Response:", {
        status: evt.detail.xhr.status,
        path: evt.detail.pathInfo.requestPath,
        response: evt.detail.xhr.response
    });
});

// Handle duplicate email alerts and navigation
function handleDuplicateEmail(response) {
    const data = JSON.parse(response);
    if (confirm('A user with this email address already exists. Press OK to open the existing record, or Press Cancel to continue editing the current record.')) {
        // User clicked OK - navigate to existing record
        htmx.ajax('GET', `/api/v1/members/${data.member_id}`, {
            target: '#member-detail',
            swap: 'innerHTML'
        });
        
        // Close the new form
        const newForm = document.querySelector('#new-form');
        if (newForm) {
            newForm.remove();
        }
        
        // Select the member in the list
        const membersList = document.querySelector('#members-list');
        if (membersList) {
            htmx.trigger(membersList, 'refreshMembersList');
            // After list refresh, select the member
            setTimeout(() => {
                const memberRow = document.querySelector(`[data-member-id="${data.member_id}"]`);
                if (memberRow) {
                    memberRow.click();
                }
            }, 100);
        }
    }
    // If user clicks Cancel, do nothing and leave them on the form
}

function toggleBilling(button) {
    const section = button.nextElementSibling;
    const icon = button.querySelector('svg');
    
    if (section.classList.contains('hidden')) {
        section.classList.remove('hidden');
        icon.classList.add('rotate-180');
        
        // Load billing info if not already loaded
        if (section.textContent.includes('Loading')) {
            const memberId = button.closest('[data-member-id]').dataset.memberId;
            htmx.ajax('GET', `/api/v1/members/${memberId}/billing`, {target: section});
        }
    } else {
        section.classList.add('hidden');
        icon.classList.remove('rotate-180');
    }
}


// Confirm and handle member deletion
function confirmDelete(id, name) {
    if (confirm(`Are you sure you want to delete ${name}?`)) {
        htmx.ajax('DELETE', `/api/v1/members/${id}`, {
            target: '#member-detail',
            swap: 'outerHTML',
            afterSwap: function() {
                // Refresh the members list
                htmx.trigger('#members-list', 'load');
            }
        });
    }
}

