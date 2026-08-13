// Shared sortable-table initialiser.
// Works with .runtime-table elements whose <th> carry data-sort and data-sort-type attributes.
// Rows must have <td data-sort-key="<col>" data-sort="<value>"> for each sortable column.

function initSortableTables(root) {
    (root || document).querySelectorAll('.runtime-table').forEach(table => {
        if (table.dataset.sortable) return;
        table.dataset.sortable = 'true';
        table.querySelectorAll('thead th[data-sort]').forEach(th => {
            th.classList.add('sortable');
            th.addEventListener('click', () => sortByColumn(table, th));
        });
    });
}

function sortByColumn(table, th) {
    const key = th.dataset.sort;
    const type = th.dataset.sortType || 'string';
    const currentKey = table.dataset.sortKey;
    let dir = 'asc';
    if (currentKey === key) {
        dir = table.dataset.sortDir === 'asc' ? 'desc' : 'asc';
    }
    table.dataset.sortKey = key;
    table.dataset.sortDir = dir;
    table.querySelectorAll('thead th').forEach(h => h.classList.remove('sorted-asc', 'sorted-desc'));
    th.classList.add(dir === 'asc' ? 'sorted-asc' : 'sorted-desc');
    const tbody = table.querySelector('tbody');
    const rows = Array.from(tbody.querySelectorAll('tr'));
    const multiplier = dir === 'asc' ? 1 : -1;
    rows.sort((a, b) => {
        const aCell = a.querySelector(`td[data-sort-key="${key}"]`);
        const bCell = b.querySelector(`td[data-sort-key="${key}"]`);
        const av = aCell ? aCell.dataset.sort : '';
        const bv = bCell ? bCell.dataset.sort : '';
        if (type === 'number') {
            return multiplier * (Number(av) - Number(bv));
        }
        return multiplier * av.localeCompare(bv);
    });
    rows.forEach(row => tbody.appendChild(row));
}

// Init on load, then re-init whenever a details panel is opened (lazy tables).
document.addEventListener('DOMContentLoaded', () => initSortableTables());
document.addEventListener('toggle', e => {
    if (e.target.tagName === 'DETAILS' && e.target.open) {
        initSortableTables(e.target);
    }
}, true);
