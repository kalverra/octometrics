document.addEventListener('DOMContentLoaded', () => {
    const input = document.getElementById('search-input');
    const container = document.getElementById('search-results-container');
    if (!input || !container) return;

    let debounceTimer;

    input.addEventListener('input', () => {
        clearTimeout(debounceTimer);
        const query = input.value.trim();
        if (query.length < 3) {
            return;
        }

        debounceTimer = setTimeout(() => {
            fetch('/search?q=' + encodeURIComponent(query) + '&partial=1')
                .then(response => {
                    if (!response.ok) throw new Error('Network error');
                    return response.text();
                })
                .then(html => {
                    container.innerHTML = html;
                })
                .catch(() => {});
        }, 250);
    });
});
