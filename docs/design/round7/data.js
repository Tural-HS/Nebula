// Synthetic fixture data for the round-7 mockups. No real people or companies.
window.NB = {
  people: { AM: "Aysel M.", RK: "Rauf K.", LH: "Leyla H.", TS: "Tofiq S.", NQ: "Nigar Q." },
  statuses: [
    { name: "Görüləcək", cat: "todo" },
    { name: "İcrada", cat: "doing" },
    { name: "Yoxlamada", cat: "doing", gate: true },
    { name: "Tamamlandı", cat: "done" },
  ],
  items: [
    { key: "FIN-212", title: "Bank çıxarışının uzlaşdırılması — oktyabr", type: "Uzlaşdırma", status: 1, who: "AM", due: "26 noy", dueRel: "2 iş günü", amount: 184250.4, signals: { subtasks: [3, 5] } },
    { key: "FIN-209", title: "Kontragent 0142 üzrə ödəniş tapşırığı", type: "Ödəniş", status: 2, who: "RK", due: "21 noy", dueRel: "3 iş günü gecikib", amount: 42800, signals: { overdue: true, approval: [1, 2] } },
    { key: "FIN-207", title: "ƏDV bəyannaməsi, III rüb", type: "Hesabat", status: 1, who: "LH", due: "28 noy", dueRel: "4 iş günü", amount: 0, signals: { blocked: true } },
    { key: "FIN-205", title: "Avans hesabatı: ezamiyyət 11/03", type: "Xərc", status: 0, who: "TS", due: "2 dek", dueRel: "6 iş günü", amount: 1240.75, signals: {} },
    { key: "FIN-204", title: "Debitor borcların yaşlanma cədvəli", type: "Hesabat", status: 2, who: "NQ", due: "25 noy", dueRel: "sabah", amount: 0, signals: { approval: [0, 2], subtasks: [2, 2] } },
    { key: "FIN-201", title: "Kassa limitinin yenilənməsi", type: "Sorğu", status: 0, who: null, due: "", dueRel: "", amount: 5000, signals: {} },
    { key: "FIN-198", title: "Əsas vəsaitlərin amortizasiyası — noyabr", type: "Jurnal", status: 1, who: "AM", due: "30 noy", dueRel: "5 iş günü", amount: 9612.3, signals: {} },
    { key: "FIN-196", title: "Kontragent 0388 ilə üzləşmə aktı", type: "Uzlaşdırma", status: 1, who: "RK", due: "20 noy", dueRel: "4 iş günü gecikib", amount: 27455.1, signals: { overdue: true, blocked: true } },
    { key: "FIN-193", title: "Valyuta məzənnə fərqlərinin uçotu", type: "Jurnal", status: 3, who: "LH", due: "18 noy", dueRel: "", amount: 318.9, signals: {} },
    { key: "FIN-190", title: "Əmək haqqı fondunun köçürülməsi", type: "Ödəniş", status: 2, who: "TS", due: "27 noy", dueRel: "3 iş günü", amount: 126300, signals: { approval: [1, 2] } },
    { key: "FIN-188", title: "Hesab-faktura reyestrinin yoxlanması", type: "Yoxlama", status: 0, who: "NQ", due: "4 dek", dueRel: "8 iş günü", amount: 0, signals: { subtasks: [0, 4] } },
    { key: "FIN-185", title: "Depozit faizlərinin hesablanması", type: "Jurnal", status: 3, who: "AM", due: "15 noy", dueRel: "", amount: 2210.55, signals: {} },
  ],
  // az-AZ: narrow no-break space between thousands, comma before decimals.
  fmt: (n) => (n ? n.toFixed(2).replace(".", ",").replace(/\B(?=(\d{3})+(?!\d))/g, "\u202f") : "—"),
};
// ?theme=dark | light, ?view=drawer (phone: open the drawer screen)
(function () {
  const p = new URLSearchParams(location.search);
  if (p.get("theme") === "dark") document.documentElement.classList.add("dark");
  if (p.get("view") === "drawer") document.documentElement.classList.add("show-drawer");
  if (p.get("view") === "list") document.documentElement.classList.add("hide-drawer");
})();
