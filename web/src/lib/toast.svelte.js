let seq = 0;

class Toasts {
  items = $state([]);

  push(message, kind = 'info', ms = 4000) {
    const id = ++seq;
    this.items.push({ id, message, kind });
    setTimeout(() => this.dismiss(id), ms);
  }
  success(m) { this.push(m, 'success'); }
  error(m) { this.push(m, 'error', 7000); }
  dismiss(id) { this.items = this.items.filter((t) => t.id !== id); }
}

export const toasts = new Toasts();
