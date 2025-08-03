// @ts-ignore
const { useState, useEffect } = React;

interface Item {
    domain: string;
    record: null | string[];
    expire: null | Date;
}

type AlertType = 'success' | 'danger';

interface Alert {
    id: Date;
    type: AlertType;
    message: string | Error;
}

// Store a copy of the fetch function
const _oldFetch = fetch;

// Create our new version of the fetch function
window.fetch = function(){

    // Create hooks
    // @ts-ignore
    const fetchStart = new Event( 'fetchStart', { 'view': document, 'bubbles': true, 'cancelable': false } );
    // @ts-ignore
    const fetchEnd = new Event( 'fetchEnd', { 'view': document, 'bubbles': true, 'cancelable': false } );

    // Pass the supplied arguments to the real fetch function
    const fetchCall = _oldFetch.apply(this, arguments);

    // Trigger the fetchStart event
    document.dispatchEvent(fetchStart);

    let called = false;

    fetchCall.then(() => {
        if (!called) {
            document.dispatchEvent(fetchEnd);
        }
    }).catch(() => {
        if (!called) {
            document.dispatchEvent(fetchEnd);
        }
    });

    return fetchCall;
};

function Page() {
    const [items, setItems] = useState([] as Item[]);
    const [value, setValue] = useState("")
    const [filter, setFilter] = useState("")
    const [uniqIPs, setUniqIPs] = useState(0)
    const [domains, setDomains] = useState(0)
    const [total, setTotal] = useState(0)
    const [loading, setLoading] = useState(0)
    const [alerts, setAlerts] = useState([] as Alert[])
    // @ts-ignore
    const [listUniqIPS, setListUniqIPS] = useState<Map<string, number>>(new Map());

    const fetchAndDownload = () => {
        aFetchData().then(data => {
            if (!data) {
                alert("Нет данных")

                return
            }

            let element = document.createElement('a');
            // @ts-ignore
            let resolve = new Set();

            data.map((item : Item)=> resolve.add(item.domain));

            // @ts-ignore
            const text = Array.from(resolve).join(",");

            element.setAttribute('href', 'data:text/plain;charset=utf-8,' + encodeURIComponent(text));
            element.setAttribute('download', 'domains.txt');

            element.style.display = 'none';
            document.body.appendChild(element);

            element.click();

            document.body.removeChild(element);
        })
    }

    const aFetchData = () => {
        return fetch("/api")
            .then(res => {
                if (res.ok) return res.json()

                return res.text().then(text => {
                    throw new Error(text || "Server error")
                })
            })
            .then(data => {
                data.list && data.list.sort((a: Item, b: Item) => {
                    const l1 = a.record ? a.record.length : 0;
                    const l2 = b.record ? b.record.length : 0;

                    if ((l2 - l1) === 0) {
                        return a.domain.localeCompare(b.domain);
                    }

                    return l2 - l1;
                })

                setItems(data.list)

                return data.list
            })
    }

    const fetchData = () => {
        return aFetchData().then(data => {
            let count = 0;
            // @ts-ignore
            let uniqIPs = new Set();
            // @ts-ignore
            let uniqDomains = new Set();
            // @ts-ignore
            let uniques = new Map<string, number>();

            data && data.map((item : Item) => {
                if (!item.record) return;

                count += item.record.length;

                item.record.forEach(item => {
                    uniqIPs.delete(item)
                    uniqIPs.add(item)
                    uniques.set(item, (uniques.get(item) ?? 0) + 1);
                })
                uniqDomains.add(item.domain)
            })

            setTotal(count)
            setUniqIPs(uniqIPs.size)
            setListUniqIPS(uniques)
            setDomains(uniqDomains.size)
        }).catch(err => pushAlert("danger", err))
    }

    const remove = (domain : string ) => {
        if (!confirm("Уверены?")) return;

        fetch(`/api/${domain}`, { method: 'DELETE' })
            .then(data => {
                if (data.ok) {
                    pushAlert("success", "Успешно удалён")

                    return fetchData()
                }

                return data.text().then(text => {
                    throw new Error(text || "Server error")
                })
            }).catch(err => pushAlert("danger", err))
    }

    const edit = (domain : string) => {
        const newDomain = prompt('Enter new domain:', domain);
        if (newDomain) {
            fetch(`/api/${domain}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ domain: newDomain }),
            }).then(res => {
                if (res.ok) {
                    pushAlert("success", "Успешно изменён")

                    return fetchData();
                }

                return res.text().then(text => {
                    throw new Error(text || "Server error")
                })
            }).catch(err => pushAlert("danger", err))
        }
    }

    function onSubmit(event: FormDataEvent) {
        event.preventDefault()

        fetch('/api', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ domain: value }),
        }).then(res => {
            if (res.ok) {
                setValue("")
                // @ts-ignore
                event.target.reset()

                pushAlert("success", "Успешно добавлен")

                return fetchData()
            }

            return res.text().then(text => {
                throw new Error(text || "Server error")
            })
        }).catch(err => pushAlert("danger", err))
    }

    // Добавить уведомление
    const pushAlert = (type: AlertType, message: any) => {
        const id = Date.now(); // Уникальный ID

        setAlerts(prev => [...prev, {id: id, type: type, message: message}]);

        // Удаление через 5 секунд
        setTimeout(() => {
            setAlerts(prev => prev.filter(n => n.id !== id));
        }, 5000);
    };

    const getMessage = (item : Alert): string =>  {
        const raw = item.message instanceof Error ? item.message.message : item.message;

        try {
            const parsed = JSON.parse(raw);

            if (typeof parsed === 'object' && parsed !== null) {
                const code = parsed.code ?? 'Unknown';
                const text = parsed.message ?? JSON.stringify(parsed);
                return `[${code}] ${text}`;
            }

            return raw;
        } catch {
            return raw;
        }
    }

    useEffect(() => {

        fetchData()

        const interval = setInterval(fetchData, 5000);

        return () => clearInterval(interval);
    }, []);

    useEffect(() => {
        const increase = () => setLoading((prev) => prev + 1);
        const decrease = () => setLoading((prev) => (prev > 0 ? prev - 1 : 0));

        document.addEventListener('fetchStart', increase);
        document.addEventListener('fetchEnd', decrease);

        return () => {
            document.removeEventListener('fetchStart', increase);
            document.removeEventListener('fetchEnd', decrease);
        };
    }, []);

    return (<div class="container-sm mx-auto table-responsive-sm">
        <h1 className="text-center h3 my-3">Управление DNS / BGP</h1>

        <form className="needs-validation position-relative" noValidate onSubmit={onSubmit}>
            <div className="input-group has-validation">
                <button className="btn btn-success" type="button" onClick={fetchData}>&#8635;</button>
                <button className="btn btn-warning" type="button" onClick={fetchAndDownload}> ↓㆔</button>
                <label className="input-group-text" htmlFor="domain-name">
                    <div className={`spinner-border text-success ${loading <= 0 ? "d-none" : ""}`} role="status">
                        <span className="visually-hidden">Loading...</span>
                    </div>

                    <span className="mx-2"> Домен </span>
                </label>
                <input required
                       type="text"
                       className="form-control"
                       value={value}
                       placeholder="Введите значение"
                       onChange={
                           // @ts-ignore
                           (event: React.FormEvent) => setValue(event.target.value)
                       }
                />
                <input type="submit" className="btn btn-primary" value=" 💾 "/>
                <div className="invalid-tooltip">
                    Введите корректный домен
                </div>
            </div>
        </form>

        <div className="mt-3">
            <svg xmlns="http://www.w3.org/2000/svg" className="d-none">
                <symbol id="check-circle-fill" viewBox="0 0 16 16">
                    <path
                        d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.06L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/>
                </symbol>
                <symbol id="info-fill" viewBox="0 0 16 16">
                    <path
                        d="M8 16A8 8 0 1 0 8 0a8 8 0 0 0 0 16zm.93-9.412-1 4.705c-.07.34.029.533.304.533.194 0 .487-.07.686-.246l-.088.416c-.287.346-.92.598-1.465.598-.703 0-1.002-.422-.808-1.319l.738-3.468c.064-.293.006-.399-.287-.47l-.451-.081.082-.381 2.29-.287zM8 5.5a1 1 0 1 1 0-2 1 1 0 0 1 0 2z"/>
                </symbol>
                <symbol id="exclamation-triangle-fill" viewBox="0 0 16 16">
                    <path
                        d="M8.982 1.566a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767L8.982 1.566zM8 5c.535 0 .954.462.9.995l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 5.995A.905.905 0 0 1 8 5zm.002 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/>
                </symbol>
            </svg>

            {alerts?.map((item: Alert) => (
                <div key={item.id} className={`alert alert-${item.type} d-flex align-items-center`} role="alert">
                    <svg className="bi flex-shrink-0 me-2" role="img" aria-label={`${item.type}:`}>
                        {item.type == "success" && (<use href="#check-circle-fill"/>)}
                        {item.type == "danger" && (<use href="#exclamation-triangle-fill"/>)}
                    </svg>
                    <div>{getMessage(item)}</div>
                </div>
            ))}
        </div>

        <table className="table table-bordered table-hover caption-top w-full" id="cache-table">
            <caption>
                <div className="row text-muted small text-center my-2">
                    <div className="col5">
                        <strong>Уникальных IP (уникальных / всего):</strong> {uniqIPs} / {total}
                    </div>
                    <div className="col5">
                        <strong>Доменов (проверенных / всего):</strong> {domains} / {items?.length ?? 0}
                    </div>
                </div>
                <div className="input-group">
                    <label className="input-group-text" htmlFor="tableFilter">Фильтр</label>
                    <input type="text"
                           className="form-control"
                           placeholder="Введите для фильтрации"
                           onChange={
                               // @ts-ignore
                               (event: React.FormEvent) => setFilter(event.target.value)
                           }
                    />
                </div>
            </caption>
            <thead className="table-warning align-middle">
            <tr>
                <th className="w-40">
                    <span className="text-nowrap">Домен</span>
                </th>
                <th className="w-auto text-center text-nowrap">Обновится</th>
                <th className="w-auto text-center text-nowrap" title='IP адреса (уникальные / всего)'>IPs<br/> (u / a)
                </th>
                <th className="w-auto text-center text-nowrap"> 🛠</th>
            </tr>
            </thead>
            <tbody className="">
            {items && items.map(item => {
                return (!filter || item.domain.includes(filter)) && (<tr key={item.domain}>
                    <td className="w-40 text-nowrap"
                        style={{overflow: "hidden", textOverflow: "ellipsis"}}>{item.domain}</td>
                    <td className="w-15 text-center">{item.expire ? (new Date(item.expire)).toLocaleString('ru-RU', {}) : "—"}</td>
                    <td className="w-10 text-center text-nowrap" title={item.record && item.record.join(",")}>
                        {item.record?.filter((key) => listUniqIPS?.get(key) <= 1).length || 0}
                        <span> / </span>
                        {item.record?.length || 0}
                    </td>
                    <td className="w-15 text-center text-nowrap">
                        <div className="input-group" style={{minWidth: '70px'}}>
                            <button type="button" className="form-control btn btn-warning btn-sm" onClick={() => {
                                edit(item.domain)
                            }}><span style={{transform: 'scaleX(-1)'}}>&#9998;</span></button>
                            <button type="button" className="form-control btn btn-danger btn-sm" onClick={() => {
                                remove(item.domain)
                            }}>&#x2715;</button>
                        </div>
                    </td>
                </tr>)
            })}
            </tbody>
        </table>
    </div>);
}

const container = document.getElementById('root');
// @ts-ignore
const root = ReactDOM.createRoot(container!);
root.render(<Page/>);