// web/static/js/members.js
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
