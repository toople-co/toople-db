function(doc) {
	if (doc.type == 'member') {
		emit([doc.user, 'circle', doc.date], {_id: doc.circle});
	}
	if (doc.type == 'participant') {
		emit([doc.user, doc.type, doc.date], {_id: doc.event});
	}
}