let htmlToImage;
let loadError = false;

try {
    htmlToImage = await import('https://cdn.jsdelivr.net/npm/html-to-image@1.11.11/+esm');
} catch (error) {
    console.error('Failed to load html-to-image', error);
    loadError = true;
}

function slugify(text) {
    return text
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '');
}

function buildFilename(scope) {
    const parts = window.location.pathname.replace(/\.html$/, '').split('/').filter(Boolean);
    const pathSlug = parts.map(slugify).join('-');
    const scopeSlug = slugify(scope);
    const prefix = pathSlug ? `octometrics-${pathSlug}` : 'octometrics';
    return `${prefix}-${scopeSlug}.png`;
}

async function exportNodeToPng(node, filename) {
    if (!htmlToImage || loadError) {
        return;
    }

    const computedBodyBg = window.getComputedStyle(document.body).backgroundColor || '#ffffff';
    const filter = (element) => {
        if (!element.classList) {
            return true;
        }
        return (
            !element.classList.contains('export-container') &&
            !element.classList.contains('export-btn') &&
            !element.classList.contains('export-menu')
        );
    };

    let wasClosed = false;
    if (node.tagName && node.tagName.toLowerCase() === 'details' && !node.open) {
        node.open = true;
        wasClosed = true;
    }

    try {
        const dataUrl = await htmlToImage.toPng(node, {
            pixelRatio: 2,
            backgroundColor: computedBodyBg,
            filter,
        });

        const link = document.createElement('a');
        link.download = filename;
        link.href = dataUrl;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    } catch (err) {
        console.error('Failed to export image', err);
    } finally {
        if (wasClosed) {
            node.open = false;
        }
    }
}

function initExportUI() {
    const header = document.querySelector('.page-header');
    if (!header) {
        return;
    }

    const container = document.createElement('div');
    container.className = 'export-container';

    const btn = document.createElement('button');
    btn.className = 'export-btn';
    btn.type = 'button';
    btn.textContent = 'Export to PNG';

    if (loadError) {
        btn.disabled = true;
        btn.title = 'Export unavailable (html-to-image CDN failed to load)';
        container.appendChild(btn);
        header.appendChild(container);
        return;
    }

    const menu = document.createElement('div');
    menu.className = 'export-menu';
    menu.hidden = true;

    const fullPageItem = document.createElement('button');
    fullPageItem.type = 'button';
    fullPageItem.className = 'export-menu-item';
    fullPageItem.textContent = 'Full page';
    fullPageItem.addEventListener('click', async () => {
        menu.hidden = true;
        const mainContainer = document.querySelector('.container') || document.body;
        await exportNodeToPng(mainContainer, buildFilename('full'));
    });
    menu.appendChild(fullPageItem);

    const eventSections = document.querySelectorAll('details.section.event-section');
    eventSections.forEach((section, idx) => {
        const summary = section.querySelector('summary');
        let label = `Event ${idx + 1}`;
        if (summary) {
            label = summary.childNodes[0]?.textContent?.trim() || summary.textContent.trim();
        }

        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'export-menu-item';
        item.textContent = label;
        item.addEventListener('click', async () => {
            menu.hidden = true;
            await exportNodeToPng(section, buildFilename(`event-${label}`));
        });
        menu.appendChild(item);
    });

    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        menu.hidden = !menu.hidden;
    });

    document.addEventListener('click', (e) => {
        if (!container.contains(e.target)) {
            menu.hidden = true;
        }
    });

    container.appendChild(btn);
    container.appendChild(menu);
    header.appendChild(container);
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initExportUI);
} else {
    initExportUI();
}
