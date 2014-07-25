function(doc) {
	if (doc.type == 'participant') {
		emit([doc.event, doc.date], {_id: doc.user});
	}
}
