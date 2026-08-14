// Shared sortable-table initialiser.
// Works with .runtime-table elements whose <th> carry data-sort and data-sort-type attributes.
// Rows must have <td data-sort-key="<col>" data-sort="<value>"> for each sortable column.

function initSortableTables(root) {
    (root || document).querySelectorAll('.runtime-table').forEach(table => {
        table.querySelectorAll('thead th[data-sort]').forEach(th => {
            th.classList.add('sortable');
        });
    });
}

function storeOriginalOrder(tbody) {
    if (tbody.dataset.hasOriginalOrder) return;
    tbody.dataset.hasOriginalOrder = 'true';
    Array.from(tbody.querySelectorAll('tr')).forEach((row, i) => {
        row.dataset.originalIndex = String(i);
    });
}

function sortByColumn(table, th) {
    const key = th.dataset.sort;
    if (!key) return;

    const tbody = table.querySelector('tbody');
    if (!tbody) return;

    storeOriginalOrder(tbody);

    const type = th.dataset.sortType || 'string';
    const currentKey = table.dataset.sortKey;
    const currentDir = table.dataset.sortDir || '';

    let dir = 'desc'; // 1st click: descending
    if (currentKey === key) {
        if (currentDir === 'desc') {
            dir = 'asc'; // 2nd click: ascending
        } else if (currentDir === 'asc') {
            dir = 'none'; // 3rd click: no sorting
        }
    }

    table.querySelectorAll('thead th').forEach(h => h.classList.remove('sorted-asc', 'sorted-desc'));

    if (dir === 'none') {
        table.dataset.sortKey = '';
        table.dataset.sortDir = '';
    } else {
        table.dataset.sortKey = key;
        table.dataset.sortDir = dir;
        th.classList.add(dir === 'desc' ? 'sorted-desc' : 'sorted-asc');
    }

    const rows = Array.from(tbody.querySelectorAll('tr'));

    if (dir === 'none') {
        rows.sort((a, b) => Number(a.dataset.originalIndex) - Number(b.dataset.originalIndex));
    } else {
        const multiplier = dir === 'asc' ? 1 : -1;
        rows.sort((a, b) => {
            const aCell = a.querySelector(`td[data-sort-key="${key}"]`);
            const bCell = b.querySelector(`td[data-sort-key="${key}"]`);
            const av = aCell ? (aCell.dataset.sort ?? '') : '';
            const bv = bCell ? (bCell.dataset.sort ?? '') : '';

            if (type === 'number') {
                const na = parseFloat(av);
                const nb = parseFloat(bv);
                if (isNaN(na) && isNaN(nb)) return 0;
                if (isNaN(na)) return 1;
                if (isNaN(nb)) return -1;
                return multiplier * (na - nb);
            }
            return multiplier * av.localeCompare(bv);
        });
    }

    rows.forEach(row => tbody.appendChild(row));
}

// Global click event delegation for sortable headers
document.addEventListener('click', e => {
    const th = e.target.closest('.runtime-table thead th[data-sort]');
    if (!th) return;
    const table = th.closest('.runtime-table');
    if (!table) return;
    sortByColumn(table, th);
});

// Run initialization immediately or on DOMContentLoaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => initSortableTables());
} else {
    initSortableTables();
}

document.addEventListener('toggle', e => {
    if (e.target.tagName === 'DETAILS' && e.target.open) {
        initSortableTables(e.target);
    }
}, true);
